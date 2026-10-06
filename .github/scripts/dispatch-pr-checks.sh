#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "Usage: $0 <head-branch> <head-sha> <base-branch> <pr-number>" >&2
  exit 2
fi

head_branch="$1"
head_sha="$2"
base_branch="$3"
pr_number="$4"
repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
workflows=("CI" "Security Scan")
declare -A previous_ids
declare -A successful_workflows
required_checks=(
  frontend
  golangci-lint
  test
  postgres-compatibility
  'test-race (proxy-a-g)'
  'test-race (proxy-h-z)'
  'test-race (admin)'
  'test-race (database)'
  'test-race (promptfilter)'
  'test-race (rest)'
  backend-security
  frontend-security
)
if [[ "$base_branch" == "codex2api-custom" ]]; then
  required_checks+=(custom-contract)
fi

latest_dispatch_id() {
  local workflow="$1"
  gh api "repos/${repository}/actions/runs?head_sha=${head_sha}&event=workflow_dispatch&per_page=100" \
    | jq -r --arg name "$workflow" --arg branch "$head_branch" --arg sha "$head_sha" '
        [.workflow_runs[] | select(.name == $name and .head_branch == $branch and .head_sha == $sha)]
        | sort_by(.created_at) | last | .id // empty'
}

for workflow in "${workflows[@]}"; do
  previous_ids["$workflow"]="$(latest_dispatch_id "$workflow")"
done

gh workflow run CI --repo "$repository" --ref "$head_branch" -f "target_branch=${base_branch}"
gh workflow run "Security Scan" --repo "$repository" --ref "$head_branch"

for attempt in $(seq 1 90); do
  runs_json=$(gh api "repos/${repository}/actions/runs?head_sha=${head_sha}&event=workflow_dispatch&per_page=100")
  all_succeeded=true

  for workflow in "${workflows[@]}"; do
    if [[ "${successful_workflows[$workflow]:-false}" == true ]]; then
      continue
    fi
    run_json=$(jq -c \
      --arg name "$workflow" \
      --arg branch "$head_branch" \
      --arg sha "$head_sha" \
      --arg previous "${previous_ids[$workflow]}" '
        [.workflow_runs[]
          | select(.name == $name and .head_branch == $branch and .head_sha == $sha)
          | select((.id | tostring) != $previous)]
        | sort_by(.created_at) | last // empty' <<< "$runs_json")

    if [[ -z "$run_json" ]]; then
      all_succeeded=false
      continue
    fi

    run_id=$(jq -r '.id' <<< "$run_json")
    status=$(jq -r '.status' <<< "$run_json")
    conclusion=$(jq -r '.conclusion // empty' <<< "$run_json")
    run_url=$(jq -r '.html_url' <<< "$run_json")

    if [[ "$status" == "completed" && "$conclusion" != "success" ]]; then
      echo "${workflow} failed for ${head_sha}: ${run_url} (${conclusion:-no conclusion})." >&2
      exit 1
    fi
    if [[ "$status" != "completed" || "$conclusion" != "success" ]]; then
      all_succeeded=false
    else
      successful_workflows["$workflow"]=true
      echo "${workflow} passed: ${run_url}"
    fi
  done

  if [[ "$all_succeeded" == true ]]; then
    pr_json=$(gh pr view "$pr_number" --repo "$repository" --json statusCheckRollup)
    checks_ready=true
    for check in "${required_checks[@]}"; do
      state=$(jq -r --arg name "$check" \
        '[.statusCheckRollup[] | select(.name == $name) | (.conclusion // .state // "")] | last // ""' \
        <<< "$pr_json")
      if [[ "$state" != "SUCCESS" && "$state" != "success" ]]; then
        checks_ready=false
        if [[ "$state" =~ ^(FAILURE|failure|CANCELLED|cancelled|TIMED_OUT|timed_out|ACTION_REQUIRED|action_required|STARTUP_FAILURE|startup_failure|ERROR|error)$ ]]; then
          echo "Required check ${check} failed on PR #${pr_number}: ${state}." >&2
          exit 1
        fi
      fi
    done
    if [[ "$checks_ready" == true ]]; then
      exit 0
    fi
  fi
  sleep 10
done

echo "Timed out waiting for CI and Security Scan on ${head_sha}." >&2
exit 1
