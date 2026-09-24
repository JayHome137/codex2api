package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/tidwall/gjson"
)

const (
	sub2RateProbeTimeout = 8 * time.Second
	sub2RateIntervalKey  = "sub2_upstream_rate_probe_interval_minutes"
)

var sub2RateIntervals = map[int64]bool{5: true, 10: true, 20: true, 30: true}

// populateSub2UpstreamCost probes on eligible account traffic, then applies the
// last known multiplier to this request's Codex2API bill.
func (h *Handler) populateSub2UpstreamCost(input *database.UsageLogInput) {
	if h == nil || input == nil || input.AccountID <= 0 || h.store == nil || h.db == nil || input.UpstreamRateMultiplier > 0 {
		return
	}
	account := h.store.FindByID(input.AccountID)
	if account == nil || !account.IsOpenAIResponsesAPI() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	row, err := h.db.GetAccountByID(ctx, input.AccountID)
	if err != nil || row == nil || !row.GetCredentialBool("sub2_upstream_rate_probe_enabled") {
		return
	}
	baseURL, apiKey := account.OpenAIResponsesCredentials()
	if baseURL == "" || apiKey == "" {
		return
	}
	now := time.Now().UTC()
	interval := int64(5)
	if configured, ok := row.GetCredentialInt64(sub2RateIntervalKey); ok && sub2RateIntervals[configured] {
		interval = configured
	}
	lastProbe, _ := time.Parse(time.RFC3339, row.GetCredential("sub2_upstream_rate_probe_at"))
	multiplier, hasRate := row.GetCredentialFloat64("sub2_upstream_rate_multiplier")
	if !lastProbe.IsZero() && now.Before(lastProbe.Add(time.Duration(interval)*time.Minute)) {
		if hasRate && multiplier > 0 {
			applySub2Rate(input, multiplier)
		}
		return
	}
	// Duplicate traffic for the same account shares one probe result.
	result, _, _ := h.sub2RateFlight.Do(strconv.FormatInt(account.DBID, 10), func() (interface{}, error) {
		checkedAt := time.Now().UTC()
		value, ok := QuerySub2EffectiveRateMultiplier(baseURL, apiKey, account.ProxyURL, account.CustomHeaders)
		updates := map[string]interface{}{
			"sub2_upstream_rate_probe_at": checkedAt.Format(time.RFC3339),
		}
		if ok {
			updates["sub2_upstream_rate_multiplier"] = value
			updates["sub2_upstream_rate_success_at"] = checkedAt.Format(time.RFC3339)
			updates["sub2_upstream_rate_probe_error"] = ""
		} else {
			updates["sub2_upstream_rate_probe_error"] = "上游未返回有效倍率"
		}
		_ = h.db.UpdateCredentials(ctx, input.AccountID, updates)
		return struct {
			multiplier float64
			ok         bool
		}{value, ok}, nil
	})
	if probed, ok := result.(struct {
		multiplier float64
		ok         bool
	}); ok && probed.ok {
		multiplier, hasRate = probed.multiplier, true
	}
	if hasRate && multiplier > 0 {
		applySub2Rate(input, multiplier)
	}
}

func applySub2Rate(input *database.UsageLogInput, multiplier float64) {
	input.UpstreamRateMultiplier = multiplier
	model := input.EffectiveModel
	if model == "" {
		model = input.Model
	}
	tier := input.BillingServiceTier
	if tier == "" {
		tier = input.ActualServiceTier
	}
	if tier == "" {
		tier = input.ServiceTier
	}
	cost, ok := database.CalculateSub2UpstreamCost(input.InputTokens, input.OutputTokens, input.CachedTokens, input.CacheWrite5mTokens, input.CacheWrite1hTokens, model, tier, multiplier)
	if !ok {
		return
	}
	input.UpstreamCost = cost.TotalCost
	input.UpstreamCostAvailable = true
	input.UpstreamCostProvider = "sub2api"
}

// QuerySub2EffectiveRateMultiplier performs a read-only upstream billing probe.
func QuerySub2EffectiveRateMultiplier(baseURL, apiKey, proxyURL string, headers map[string]string) (float64, bool) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
	if err := auth.ConfigureTransportProxy(transport, proxyURL, dialer); err != nil {
		return 0, false
	}
	client := &http.Client{Transport: transport, Timeout: sub2RateProbeTimeout}
	ctx, cancel := context.WithTimeout(context.Background(), sub2RateProbeTimeout)
	defer cancel()
	for _, path := range []string{"/v1/sub2api/billing", "/v1/usage"} {
		endpoint := auth.OpenAIResponsesEndpoint(baseURL, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")
		for name, value := range headers {
			if name = strings.TrimSpace(name); name != "" {
				req.Header.Set(name, value)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if multiplier, ok := parseSub2EffectiveRateMultiplier(body); ok {
			return multiplier, true
		}
	}
	return 0, false
}

func parseSub2EffectiveRateMultiplier(body []byte) (float64, bool) {
	for _, path := range []string{
		"effective_rate_multiplier", "rate_multiplier", "data.effective_rate_multiplier",
		"data.rate_multiplier", "billing.effective_rate_multiplier", "data.billing.effective_rate_multiplier",
	} {
		value := gjson.GetBytes(body, path)
		if value.Exists() {
			multiplier := value.Float()
			if multiplier > 0 {
				return multiplier, true
			}
		}
	}
	return 0, false
}
