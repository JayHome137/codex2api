#!/usr/bin/env bash

set -euo pipefail

custom_remove_paths=(
  frontend/src/lib/channelMonitorBilling.ts
  frontend/src/lib/channelMonitorBilling.test.mjs
)

contract_failed=false
for path in "${custom_remove_paths[@]}"; do
  if [[ -e "$path" ]]; then
    echo "Custom contract violation: forbidden file exists: $path" >&2
    contract_failed=true
  fi
done

if rg -n --hidden --glob '!.git/**' --glob '!CHANGELOG.md' --glob '!docs/**' \
  --glob '!test-results/**' --glob '!.github/workflows/upstream-sync.yml' \
  --glob '!.github/scripts/verify-custom-contract.sh' \
  -e 'ChannelMonitorBilling' \
  -e 'channelMonitorBilling' \
  -e 'BillingRates' \
  -e 'probeResponsesBilling' \
  -e 'RecordChannelMonitorBilling' \
  -e '/channel-monitors/billing-rates' .; then
  echo "Custom contract violation: upstream channel billing-rate implementation is present" >&2
  contract_failed=true
fi

required_contracts=(
  'deploy.sh|https://github.com/JayHome137/codex2api.git'
  'deploy.sh|REPO_BRANCH="${CODEX2API_REPO_BRANCH:-codex2api-custom}"'
  'docker-compose.yml|ghcr.io/jayhome137/codex2api:latest'
  'docker-compose.sqlite.yml|ghcr.io/jayhome137/codex2api:latest'
  'admin/channel_monitor.go|func (h *Handler) ListChannelMonitors'
  'admin/channel_monitor.go|func (h *Handler) ProbeChannelMonitorNow'
  'database/channel_monitor.go|CREATE TABLE IF NOT EXISTS channel_monitor_configs'
  'database/channel_monitor.go|CREATE TABLE IF NOT EXISTS channel_monitor_checks'
  'frontend/src/pages/ChannelMonitor.tsx|ChannelMonitor'
  'proxy/sub2_upstream_probe.go|/v1/sub2api/billing'
  'proxy/sub2_upstream_probe.go|/v1/usage'
  'proxy/sub2_upstream_probe.go|parseSub2BillingRateMultiplier'
  'proxy/handler.go|populateSub2UpstreamCost'
  'frontend/src/components/ui/tooltip.tsx|function ResponsiveTooltip'
  'frontend/src/components/ui/tooltip.tsx|ResponsiveTooltip }'
  'frontend/src/pages/APIKeyUsagePortal.tsx|<ResponsiveTooltip'
  'frontend/src/pages/Usage.tsx|<ResponsiveTooltip'
)

for contract in "${required_contracts[@]}"; do
  IFS='|' read -r path expected <<< "$contract"
  if ! rg -Fq -- "$expected" "$path"; then
    echo "Custom contract violation: expected '$expected' in $path" >&2
    contract_failed=true
  fi
done

if [[ "$contract_failed" == true ]]; then
  exit 1
fi

echo "Custom contract checks passed."
