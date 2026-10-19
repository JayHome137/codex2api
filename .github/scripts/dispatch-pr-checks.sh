#!/usr/bin/env bash
# Custom-contract path catalog and upstream-sync PR check helper.
#
# Source this file to load the path arrays and helpers. Execute it to wait
# for PR checks, mirror workflow jobs onto commit statuses, reuse an
# unchanged sync branch, or run --self-test.
#
# GITHUB_TOKEN recursion guard: when this token opens or updates a pull
# request, GitHub still creates the pull_request CI / Security Scan runs,
# but they complete immediately as action_required with zero jobs until a
# maintainer clicks "Approve workflows to run". Those runs do not report
# the required checks. workflow_dispatch jobs are also ignored by branch
# protection (they never appear in the PR check rollup). Commit statuses
# posted for the same check names do satisfy required checks. There is no
# repository setting that turns the approval gate off. Set the
# UPSTREAM_SYNC_TOKEN secret (fine-grained PAT or GitHub App installation
# token with contents, pull requests, and actions write) to push and open
# the PR as a normal writer so the pull_request workflows run by themselves.

# Keep the custom version when merging main into codex2api-custom.
# Every path here is also a release exception. package-lock is not listed:
# on conflict the main baseline (including its source-map-js patch) wins.
custom_preserve_paths=(
  .dockerignore
  .github/scripts/dispatch-pr-checks.sh
  .github/scripts/patch-source-map-js-lock.mjs
  .github/scripts/verify-custom-contract.sh
  .github/workflows/cleanup-sync-branch.yml
  .github/workflows/pr-check.yml
  .github/workflows/release.yml
  .github/workflows/security-scan.yml
  .github/workflows/upstream-custom-sync.yml
  .github/workflows/upstream-release.yml
  .github/workflows/upstream-sync.yml
  admin/account_response_builder.go
  admin/channel_monitor.go
  admin/channel_monitor_probe.go
  admin/channel_monitor_probe_test.go
  admin/handler.go
  admin/sub2_upstream_rate.go
  admin/system_update.go
  database/api_key_usage.go
  database/billing.go
  database/billing_test.go
  database/channel_monitor.go
  database/channel_monitor_test.go
  database/image_user_billing_test.go
  database/postgres.go
  database/sqlite.go
  database/sub2_upstream_billing.go
  database/sub2_upstream_billing_test.go
  database/usage_compaction_history_test.go
  deploy.sh
  docker-compose.sqlite.yml
  docker-compose.yml
  docs/custom-maintenance.md
  docs/sub2api-upstream-billing-phase1.md
  frontend/src/api.ts
  frontend/src/components/AccountQuickConfigSheet.tsx
  frontend/src/components/ChannelMonitorConfigDialog.tsx
  frontend/src/components/Layout.tsx
  frontend/src/components/ui/tooltip.tsx
  frontend/src/lib/accountQuickConfig.ts
  frontend/src/locales/en.json
  frontend/src/locales/zh.json
  frontend/src/pages/APIKeyUsagePortal.tsx
  frontend/src/pages/Accounts.tsx
  frontend/src/pages/ChannelMonitor.tsx
  frontend/src/pages/Usage.tsx
  frontend/src/types.ts
  proxy/handler.go
  proxy/native_oauth_routes_test.go
  proxy/official_model_pricing.go
  proxy/official_model_pricing_test.go
  proxy/sub2_upstream_probe.go
  proxy/sub2_upstream_probe_test.go
)

# Deleted on the custom branch. Not preserved: a conflict must not restore
# them, and the sync step removes them again after resolution.
custom_remove_paths=(
  frontend/src/lib/channelMonitorBilling.ts
  frontend/src/lib/channelMonitorBilling.test.mjs
)

# Allowed to differ from the upstream release tag. Includes preserved
# custom files, the release marker, the reviewed lockfile patch, and the
# channel-monitor billing files that the fork deletes.
custom_release_exclude_paths=(
  "${custom_preserve_paths[@]}"
  .upstream-release
  frontend/package-lock.json
  "${custom_remove_paths[@]}"
)

# Copied from codex2api-custom onto the main baseline. The scheduled sync
# runs the workflow file that is on main, so fork automation has to land
# there. Runtime customizations are not copied.
custom_baseline_paths=(
  .github/scripts/dispatch-pr-checks.sh
  .github/scripts/patch-source-map-js-lock.mjs
  .github/scripts/verify-custom-contract.sh
  .github/workflows/cleanup-sync-branch.yml
  .github/workflows/pr-check.yml
  .github/workflows/release.yml
  .github/workflows/security-scan.yml
  .github/workflows/upstream-custom-sync.yml
  .github/workflows/upstream-release.yml
  .github/workflows/upstream-sync.yml
)

# Job names required on both protected branches. custom-contract is added
# only for codex2api-custom.
required_ci_check_names=(
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
required_custom_check_names=(
  custom-contract
  "${required_ci_check_names[@]}"
)

configure_upstream_sync_token() {
  if [[ -z "${UPSTREAM_SYNC_TOKEN:-}" ]]; then
    return 0
  fi
  # actions/checkout installs a GITHUB_TOKEN extraheader that would
  # otherwise keep authenticating the push as the bot.
  git config --unset-all http.https://github.com/.extraheader || true
  git remote set-url origin "https://x-access-token:${UPSTREAM_SYNC_TOKEN}@github.com/${GITHUB_REPOSITORY}.git"
  export GH_TOKEN="$UPSTREAM_SYNC_TOKEN"
}

path_in_list() {
  local needle="$1"
  shift
  local candidate
  for candidate in "$@"; do
    [[ "$candidate" == "$needle" ]] && return 0
  done
  return 1
}

load_required_checks() {
  local base="$1"
  if [[ "$base" == "codex2api-custom" ]]; then
    required_checks=("${required_custom_check_names[@]}")
  else
    required_checks=("${required_ci_check_names[@]}")
  fi
}

is_hard_failure() {
  case "${1,,}" in
    failure|cancelled|timed_out|action_required|startup_failure|error) return 0 ;;
    *) return 1 ;;
  esac
}

is_success_state() {
  case "${1,,}" in
    success|skipped|neutral) return 0 ;;
    *) return 1 ;;
  esac
}

# Print required checks from a rollup JSON array whose conclusion/state is a
# hard failure. Approval-gated workflow shells are ignored unless their name
# is itself a required check.
failed_required_from_rollup_json() {
  local rollup_json="$1"
  shift
  local failed_checks=() check state joined=""
  for check in "$@"; do
    state=$(jq -r --arg name "$check" '
      [.[]?
        | select((.name // .context // "") == $name)
        | (.conclusion // .state // "")]
      | last // ""' <<<"$rollup_json")
    if is_hard_failure "$state"; then
      failed_checks+=("$check")
    fi
  done
  for check in "${failed_checks[@]}"; do
    if [[ -z "$joined" ]]; then
      joined="$check"
    else
      joined+=", ${check}"
    fi
  done
  printf '%s' "$joined"
}

missing_success_statuses() {
  local statuses_json="$1"
  shift
  local missing=() check state
  for check in "$@"; do
    state=$(jq -r --arg name "$check" '
      [.statuses[]? | select(.context == $name) | .state] | first // ""' <<<"$statuses_json")
    if ! is_success_state "$state"; then
      missing+=("$check")
    fi
  done
  printf '%s\n' "${missing[@]}"
}

reuse_or_push_branch() {
  local branch="$1"
  local message="$2"
  local candidate_tree existing_sha existing_tree head_sha
  candidate_tree=$(git write-tree)
  existing_sha=$(git ls-remote origin "refs/heads/${branch}" | awk 'NR == 1 { print $1 }')
  if [[ -n "$existing_sha" ]]; then
    git fetch --no-tags origin "refs/heads/${branch}:refs/remotes/origin/${branch}"
    existing_tree=$(git rev-parse "refs/remotes/origin/${branch}^{tree}")
    if [[ "$existing_tree" == "$candidate_tree" ]]; then
      echo "Remote ${branch} already has this tree (${existing_sha}); not pushing a new commit."
      git switch -f -C "$branch" "refs/remotes/origin/${branch}"
      return 0
    fi
  fi
  if ! git diff --cached --quiet; then
    git commit -m "$message"
  fi
  head_sha=$(git rev-parse HEAD)
  if [[ -n "$existing_sha" && "$head_sha" == "$existing_sha" ]]; then
    echo "Nothing new to push for ${branch}."
    return 0
  fi
  if [[ -n "$existing_sha" ]]; then
    git push --force-with-lease="refs/heads/${branch}:${existing_sha}" origin "HEAD:refs/heads/${branch}"
  else
    git push origin "HEAD:refs/heads/${branch}"
  fi
}

publish_statuses() {
  local jobs_json pending=1 attempt name conclusion state description
  : "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
  : "${GITHUB_SHA:?GITHUB_SHA is required}"
  : "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  for attempt in $(seq 1 6); do
    jobs_json=$(gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}/jobs?per_page=100")
    pending=$(jq -r '[.jobs[] | select(.name != "publish-check-status" and .status != "completed")] | length' <<<"$jobs_json")
    if [[ "$pending" == "0" ]]; then
      break
    fi
    sleep 5
  done
  if [[ "$pending" != "0" ]]; then
    echo "Workflow jobs are still running; not mirroring a partial result." >&2
    exit 1
  fi
  while IFS=$'\t' read -r name conclusion; do
    [[ -z "$name" ]] && continue
    if ! path_in_list "$name" "${required_custom_check_names[@]}"; then
      continue
    fi
    case "$conclusion" in
      success) state=success ;;
      skipped|"") continue ;;
      *) state=failure ;;
    esac
    description="Mirrored from workflow run ${GITHUB_RUN_ID} (${conclusion})"
    gh api --method POST "repos/${GITHUB_REPOSITORY}/statuses/${GITHUB_SHA}" \
      -f state="$state" \
      -f context="$name" \
      -f description="$description" \
      -f target_url="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}" \
      >/dev/null
    echo "Published commit status ${name}=${state}"
  done < <(jq -r '.jobs[] | select(.name != "publish-check-status") | [.name, (.conclusion // "")] | @tsv' <<<"$jobs_json")
}

self_test() {
  local self_test_failed=0 path candidate count
  local -A seen=()
  for path in "${custom_preserve_paths[@]}"; do
    if [[ -n "${seen[$path]:-}" ]]; then
      echo "duplicate preserve path: $path" >&2
      self_test_failed=1
    fi
    seen[$path]=1
    if [[ ! -e "$path" ]]; then
      echo "preserve path is missing from the tree: $path" >&2
      self_test_failed=1
    fi
    if ! path_in_list "$path" "${custom_release_exclude_paths[@]}"; then
      echo "preserve path is not a release exception: $path" >&2
      self_test_failed=1
    fi
  done
  for path in "${custom_remove_paths[@]}"; do
    if [[ -e "$path" ]]; then
      echo "removed custom path still exists: $path" >&2
      self_test_failed=1
    fi
    if path_in_list "$path" "${custom_preserve_paths[@]}"; then
      echo "removed path is also preserved: $path" >&2
      self_test_failed=1
    fi
    if ! path_in_list "$path" "${custom_release_exclude_paths[@]}"; then
      echo "removed path is not a release exception: $path" >&2
      self_test_failed=1
    fi
  done
  for path in frontend/package-lock.json .upstream-release; do
    if path_in_list "$path" "${custom_preserve_paths[@]}"; then
      echo "$path must follow main on conflict, not the custom side" >&2
      self_test_failed=1
    fi
    if ! path_in_list "$path" "${custom_release_exclude_paths[@]}"; then
      echo "$path must stay a release exception" >&2
      self_test_failed=1
    fi
  done
  for path in \
    proxy/official_model_pricing.go \
    proxy/official_model_pricing_test.go \
    proxy/native_oauth_routes_test.go \
    docs/custom-maintenance.md
  do
    if ! path_in_list "$path" "${custom_preserve_paths[@]}"; then
      echo "critical custom path is not preserved: $path" >&2
      self_test_failed=1
    fi
  done
  for path in "${custom_baseline_paths[@]}"; do
    if [[ ! -e "$path" ]]; then
      echo "baseline automation path is missing: $path" >&2
      self_test_failed=1
    fi
    if ! path_in_list "$path" "${custom_preserve_paths[@]}"; then
      echo "baseline path is not preserved: $path" >&2
      self_test_failed=1
    fi
  done
  if ! grep -q '^## Custom maintenance$' docs/custom-maintenance.md \
    || ! grep -q '^## 自定义维护内容$' docs/custom-maintenance.md; then
    echo "docs/custom-maintenance.md is missing a release-note heading" >&2
    self_test_failed=1
  fi
  count=$(printf '%s\n' "${required_ci_check_names[@]}" | sort | uniq -d | wc -l | tr -d ' ')
  if [[ "$count" != "0" ]]; then
    echo "duplicate required check names" >&2
    self_test_failed=1
  fi

  local rollup='[
    {"name":"CI","conclusion":"action_required"},
    {"name":"frontend","conclusion":"SUCCESS"},
    {"context":"test","state":"FAILURE"}
  ]'
  local found
  found=$(failed_required_from_rollup_json "$rollup" frontend test golangci-lint)
  if [[ "$found" != "test" ]]; then
    echo "rollup failure parser returned '${found}', expected 'test'" >&2
    self_test_failed=1
  fi
  local statuses='{"statuses":[
    {"context":"frontend","state":"success"},
    {"context":"test","state":"pending"}
  ]}'
  found=$(missing_success_statuses "$statuses" frontend test)
  if [[ "$found" != "test" ]]; then
    echo "status parser returned '${found}', expected 'test'" >&2
    self_test_failed=1
  fi

  if git rev-parse --verify --quiet upstream-v3.1.0 >/dev/null; then
    local missing=()
    while IFS= read -r path; do
      [[ -z "$path" ]] && continue
      if ! path_in_list "$path" "${custom_release_exclude_paths[@]}"; then
        missing+=("$path")
      fi
    done < <(git diff --name-only upstream-v3.1.0 HEAD)
    if ((${#missing[@]} > 0)); then
      echo "HEAD differs from upstream-v3.1.0 outside the release contract:" >&2
      printf ' - %s\n' "${missing[@]}" >&2
      self_test_failed=1
    fi
  fi

  if [[ "$self_test_failed" != "0" ]]; then
    exit 1
  fi
  echo "Custom contract path self-test passed."
}

approval_note_printed=false
note_approval_gate() {
  if [[ "$approval_note_printed" == true ]]; then
    return 0
  fi
  approval_note_printed=true
  cat >&2 <<'EOF'
The pull_request CI and Security Scan runs are approval-gated. GitHub creates
them with conclusion action_required and zero jobs when GITHUB_TOKEN opens or
updates the PR, and workflow_dispatch jobs do not satisfy branch protection.
Commit statuses mirrored from those jobs do. No repository setting disables
this gate. A maintainer can click "Approve workflows to run", or set the
UPSTREAM_SYNC_TOKEN secret so the sync push is not authenticated as GITHUB_TOKEN.
EOF
}

runs_for_sha() {
  gh api "repos/${repository}/actions/runs?head_sha=${head_sha}&per_page=100"
}

latest_run() {
  local runs_json="$1" workflow="$2" event="$3"
  jq -c --arg name "$workflow" --arg event "$event" --arg sha "$head_sha" --arg branch "$head_branch" '
    [.workflow_runs[]
      | select(.name == $name and .event == $event and .head_sha == $sha and .head_branch == $branch)]
    | sort_by(.created_at) | last // empty' <<<"$runs_json"
}

run_job_count() {
  local run_id="$1"
  gh api "repos/${repository}/actions/runs/${run_id}/jobs?per_page=1" --jq '.total_count'
}

is_approval_gated_run() {
  local run_json="$1"
  local conclusion run_id jobs
  [[ -n "$run_json" ]] || return 1
  conclusion=$(jq -r '.conclusion // empty' <<<"$run_json")
  [[ "$conclusion" == "action_required" ]] || return 1
  run_id=$(jq -r '.id' <<<"$run_json")
  jobs=$(run_job_count "$run_id")
  [[ "$jobs" == "0" ]]
}

rollup_json() {
  local number="${1:-${pr_number:-}}"
  local pr_json rollup
  pr_json=$(gh pr view "$number" --repo "$repository" --json statusCheckRollup)
  rollup=$(jq -c '.statusCheckRollup // []' <<<"$pr_json")
  if [[ "$rollup" == "null" ]]; then
    rollup='[]'
  fi
  printf '%s' "$rollup"
}

rollup_is_green() {
  local rollup="$1" check state
  for check in "${required_checks[@]}"; do
    state=$(jq -r --arg name "$check" '
      [.[]?
        | select((.name // .context // "") == $name)
        | (.conclusion // .state // "")]
      | last // ""' <<<"$rollup")
    if ! is_success_state "$state"; then
      return 1
    fi
  done
  return 0
}

statuses_are_green() {
  local statuses_json missing
  statuses_json=$(gh api "repos/${repository}/commits/${head_sha}/status")
  missing=$(missing_success_statuses "$statuses_json" "${required_checks[@]}")
  [[ -z "$missing" ]]
}

wait_for_pr_checks() {
  local workflows=("CI" "Security Scan")
  local workflow run_json conclusion status run_url event candidate=""
  local dispatch_needed eligible_running eligible_failed
  local -A previous_dispatch_ids=()
  local -A reuse_dispatch=()
  local -A announced_dispatch=()
  local attempt dispatch_green_streak=0 all_dispatch_done run_id

  if [[ $# -ne 4 ]]; then
    echo "Usage: $0 <head-branch> <head-sha> <base-branch> <pr-number>" >&2
    exit 2
  fi
  head_branch="$1"
  head_sha="$2"
  base_branch="$3"
  pr_number="$4"
  repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  load_required_checks "$base_branch"

  if statuses_are_green || rollup_is_green "$(rollup_json)"; then
    echo "Required checks are already satisfied on ${head_sha}."
    return 0
  fi

  dispatch_needed=false
  eligible_running=false
  eligible_failed=false
  runs_json=$(runs_for_sha)
  for workflow in "${workflows[@]}"; do
    run_json=""
    for event in pull_request push pull_request_target pull_request_review; do
      candidate=$(latest_run "$runs_json" "$workflow" "$event")
      if [[ -n "$candidate" ]]; then
        run_json="$candidate"
        break
      fi
    done
    if [[ -z "$run_json" ]] || is_approval_gated_run "$run_json"; then
      dispatch_needed=true
      if [[ -n "$run_json" ]]; then
        note_approval_gate
      fi
      continue
    fi
    conclusion=$(jq -r '.conclusion // empty' <<<"$run_json")
    status=$(jq -r '.status' <<<"$run_json")
    run_url=$(jq -r '.html_url' <<<"$run_json")
    if [[ "$status" == "completed" && "$conclusion" != "success" ]]; then
      echo "${workflow} failed for ${head_sha}: ${run_url} (${conclusion:-no conclusion})." >&2
      eligible_failed=true
    elif [[ "$status" != "completed" ]]; then
      eligible_running=true
    fi
  done
  if [[ "$eligible_failed" == true ]]; then
    exit 1
  fi

  if [[ "$dispatch_needed" == false && "$eligible_running" == false ]]; then
    for attempt in $(seq 1 12); do
      if rollup_is_green "$(rollup_json)"; then
        echo "Required pull_request checks are green on ${head_sha}."
        return 0
      fi
      sleep 5
    done
    echo "Eligible workflows succeeded but the PR rollup is missing required checks." >&2
    exit 1
  fi

  if [[ "$dispatch_needed" == true ]]; then
    runs_json=$(runs_for_sha)
    for workflow in "${workflows[@]}"; do
      run_json=$(latest_run "$runs_json" "$workflow" workflow_dispatch)
      previous_dispatch_ids["$workflow"]=$(jq -r '.id // empty' <<<"${run_json:-null}")
      status=$(jq -r '.status // empty' <<<"${run_json:-null}")
      conclusion=$(jq -r '.conclusion // empty' <<<"${run_json:-null}")
      # A second workflow_dispatch on the same ref cancels the in-progress one.
      if [[ "$status" == "queued" || "$status" == "in_progress" || "$status" == "waiting" || "$status" == "pending" || "$status" == "requested" || ( "$status" == "completed" && "$conclusion" == "success" ) ]]; then
        reuse_dispatch["$workflow"]=1
      else
        reuse_dispatch["$workflow"]=0
      fi
    done
    if [[ "${reuse_dispatch[CI]:-0}" != 1 ]]; then
      gh workflow run CI --repo "$repository" --ref "$head_branch" -f "target_branch=${base_branch}"
    fi
    if [[ "${reuse_dispatch[Security Scan]:-0}" != 1 ]]; then
      gh workflow run "Security Scan" --repo "$repository" --ref "$head_branch"
    fi
  fi

  for attempt in $(seq 1 120); do
    if statuses_are_green || rollup_is_green "$(rollup_json)"; then
      echo "Required checks are satisfied on ${head_sha}."
      return 0
    fi

    if [[ "$eligible_running" == true || "$dispatch_needed" == true ]]; then
      runs_json=$(runs_for_sha)
    fi

    if [[ "$eligible_running" == true ]]; then
      eligible_running=false
      for workflow in "${workflows[@]}"; do
        run_json=""
        for event in pull_request push pull_request_target pull_request_review; do
          candidate=$(latest_run "$runs_json" "$workflow" "$event")
          if [[ -n "$candidate" ]]; then
            run_json="$candidate"
            break
          fi
        done
        [[ -n "$run_json" ]] || continue
        if is_approval_gated_run "$run_json"; then
          continue
        fi
        conclusion=$(jq -r '.conclusion // empty' <<<"$run_json")
        status=$(jq -r '.status' <<<"$run_json")
        run_url=$(jq -r '.html_url' <<<"$run_json")
        if [[ "$status" == "completed" && "$conclusion" != "success" ]]; then
          echo "${workflow} failed for ${head_sha}: ${run_url} (${conclusion:-no conclusion})." >&2
          exit 1
        fi
        if [[ "$status" != "completed" ]]; then
          eligible_running=true
        fi
      done
    fi

    if [[ "$dispatch_needed" == true ]]; then
      all_dispatch_done=true
      for workflow in "${workflows[@]}"; do
        run_json=$(latest_run "$runs_json" "$workflow" workflow_dispatch)
        run_id=$(jq -r '.id // empty' <<<"${run_json:-null}")
        if [[ "${reuse_dispatch[$workflow]:-0}" != 1 ]]; then
          # Ignore the run that existed before this dispatch; it is not the new one.
          if [[ -z "$run_id" || "$run_id" == "${previous_dispatch_ids[$workflow]:-}" ]]; then
            all_dispatch_done=false
            continue
          fi
        elif [[ -z "$run_json" ]]; then
          all_dispatch_done=false
          continue
        fi
        conclusion=$(jq -r '.conclusion // empty' <<<"$run_json")
        status=$(jq -r '.status' <<<"$run_json")
        run_url=$(jq -r '.html_url' <<<"$run_json")
        if [[ "$status" == "completed" && "$conclusion" != "success" ]]; then
          echo "${workflow} failed for ${head_sha}: ${run_url} (${conclusion:-no conclusion})." >&2
          exit 1
        fi
        if [[ "$status" != "completed" || "$conclusion" != "success" ]]; then
          all_dispatch_done=false
        elif [[ -z "${announced_dispatch[$workflow]:-}" ]]; then
          announced_dispatch["$workflow"]=1
          echo "${workflow} workflow_dispatch passed: ${run_url} (waiting for commit statuses or the PR rollup)"
        fi
      done
      if [[ "$all_dispatch_done" == true ]]; then
        dispatch_green_streak=$((dispatch_green_streak + 1))
      else
        dispatch_green_streak=0
      fi
      # The publisher job is part of the workflow run, so statuses should exist
      # once the run is green. Allow a short API lag, then fail instead of
      # waiting out the whole timeout on a rollup that will stay empty.
      if [[ "$dispatch_green_streak" -ge 6 ]]; then
        statuses_json=$(gh api "repos/${repository}/commits/${head_sha}/status")
        missing=$(missing_success_statuses "$statuses_json" "${required_checks[@]}")
        if [[ -n "$missing" ]]; then
          echo "workflow_dispatch succeeded but these required commit statuses are not success: ${missing//$'\n'/, }" >&2
          note_approval_gate
          exit 1
        fi
      fi
    fi

    if (( attempt % 12 == 0 )); then
      echo "Still waiting for required checks on ${head_sha} (attempt ${attempt}/120)."
    fi
    sleep 10
  done

  note_approval_gate
  echo "Timed out waiting for required checks on ${head_sha} to become mergeable." >&2
  exit 1
}

report_failed_required_checks() {
  local pr_number="$1" base_branch="$2" rollup
  repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  load_required_checks "$base_branch"
  rollup=$(rollup_json "$pr_number")
  failed_required_from_rollup_json "$rollup" "${required_checks[@]}"
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

set -euo pipefail

case "${1:-}" in
  --self-test)
    self_test
    ;;
  --publish-statuses)
    publish_statuses
    ;;
  --reuse-or-push)
    if [[ $# -ne 3 ]]; then
      echo "Usage: $0 --reuse-or-push <branch> <commit-message>" >&2
      exit 2
    fi
    reuse_or_push_branch "$2" "$3"
    ;;
  --failed-required-checks)
    if [[ $# -ne 3 ]]; then
      echo "Usage: $0 --failed-required-checks <pr-number> <base-branch>" >&2
      exit 2
    fi
    report_failed_required_checks "$2" "$3"
    ;;
  *)
    wait_for_pr_checks "$@"
    ;;
esac
