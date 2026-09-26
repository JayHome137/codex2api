package proxy

import (
	"context"
	"encoding/json"
	"io"
	"math"
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
		var multiplier float64
		var ok bool
		if path == "/v1/sub2api/billing" {
			multiplier, ok = parseSub2BillingRateMultiplier(body)
		} else {
			multiplier, ok = parseSub2UsageRateMultiplier(body)
		}
		if ok {
			return multiplier, true
		}
	}
	return 0, false
}

func parseSub2EffectiveRateMultiplier(body []byte) (float64, bool) {
	if multiplier, ok := parseSub2BillingRateMultiplier(body); ok {
		return multiplier, true
	}
	return parseSub2UsageRateMultiplier(body)
}

type sub2BillingWire struct {
	Object                  string   `json:"object"`
	SchemaVersion           int      `json:"schema_version"`
	BillingScope            string   `json:"billing_scope"`
	GroupRateMultiplier     *float64 `json:"group_rate_multiplier"`
	UserRateMultiplier      *float64 `json:"user_rate_multiplier"`
	ResolvedRateMultiplier  *float64 `json:"resolved_rate_multiplier"`
	PeakRateEnabled         *bool    `json:"peak_rate_enabled"`
	PeakStart               *string  `json:"peak_start"`
	PeakEnd                 *string  `json:"peak_end"`
	PeakRateMultiplier      *float64 `json:"peak_rate_multiplier"`
	AppliedPeakMultiplier   *float64 `json:"applied_peak_multiplier"`
	EffectiveRateMultiplier *float64 `json:"effective_rate_multiplier"`
	Timezone                *string  `json:"timezone"`
	ObservedAt              string   `json:"observed_at"`
}

func parseSub2BillingRateMultiplier(body []byte) (float64, bool) {
	var wire sub2BillingWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return 0, false
	}
	if wire.Object != "sub2api.key_billing" || wire.SchemaVersion != 1 || wire.BillingScope != "token" {
		return 0, false
	}
	if wire.GroupRateMultiplier == nil || wire.ResolvedRateMultiplier == nil ||
		wire.PeakRateEnabled == nil || wire.EffectiveRateMultiplier == nil {
		return 0, false
	}
	for _, value := range []*float64{
		wire.GroupRateMultiplier,
		wire.ResolvedRateMultiplier,
		wire.EffectiveRateMultiplier,
	} {
		if !validSub2Multiplier(*value) {
			return 0, false
		}
	}
	if wire.UserRateMultiplier != nil && !validSub2Multiplier(*wire.UserRateMultiplier) {
		return 0, false
	}
	expectedResolved := *wire.GroupRateMultiplier
	if wire.UserRateMultiplier != nil {
		expectedResolved = *wire.UserRateMultiplier
	}
	if !equalSub2Multiplier(*wire.ResolvedRateMultiplier, expectedResolved) {
		return 0, false
	}
	observedAt, err := time.Parse(time.RFC3339Nano, wire.ObservedAt)
	if err != nil || observedAt.IsZero() {
		return 0, false
	}
	appliedPeak := 1.0
	if *wire.PeakRateEnabled {
		if wire.PeakStart == nil || wire.PeakEnd == nil || wire.Timezone == nil ||
			wire.PeakRateMultiplier == nil || wire.AppliedPeakMultiplier == nil ||
			strings.TrimSpace(*wire.PeakStart) == "" || strings.TrimSpace(*wire.PeakEnd) == "" ||
			strings.TrimSpace(*wire.Timezone) == "" || !validSub2Multiplier(*wire.PeakRateMultiplier) ||
			!validSub2Multiplier(*wire.AppliedPeakMultiplier) {
			return 0, false
		}
		startMinute, startOK := parseSub2BillingMinute(*wire.PeakStart)
		endMinute, endOK := parseSub2BillingMinute(*wire.PeakEnd)
		if !startOK || !endOK || startMinute >= endMinute {
			return 0, false
		}
		location, locationErr := time.LoadLocation(strings.TrimSpace(*wire.Timezone))
		if locationErr != nil {
			return 0, false
		}
		localTime := observedAt.In(location)
		minute := localTime.Hour()*60 + localTime.Minute()
		if minute >= startMinute && minute < endMinute {
			appliedPeak = *wire.PeakRateMultiplier
		}
		if !equalSub2Multiplier(*wire.AppliedPeakMultiplier, appliedPeak) {
			return 0, false
		}
	} else if wire.AppliedPeakMultiplier != nil {
		if !validSub2Multiplier(*wire.AppliedPeakMultiplier) || !equalSub2Multiplier(*wire.AppliedPeakMultiplier, 1) {
			return 0, false
		}
	}
	if !equalSub2Multiplier(*wire.EffectiveRateMultiplier, *wire.ResolvedRateMultiplier*appliedPeak) {
		return 0, false
	}
	return *wire.EffectiveRateMultiplier, true
}

func parseSub2BillingMinute(value string) (int, bool) {
	colon := strings.IndexByte(value, ':')
	if (colon != 1 && colon != 2) || len(value)-colon-1 != 2 {
		return 0, false
	}
	hour, errHour := strconv.Atoi(value[:colon])
	minute, errMinute := strconv.Atoi(value[colon+1:])
	if errHour != nil || errMinute != nil || hour > 23 || minute > 59 ||
		!sub2BillingDigits(value[:colon]) || !sub2BillingDigits(value[colon+1:]) {
		return 0, false
	}
	return hour*60 + minute, true
}

func sub2BillingDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}

func equalSub2Multiplier(left, right float64) bool {
	if !validSub2Multiplier(left) || !validSub2Multiplier(right) {
		return false
	}
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-9*scale
}

func parseSub2UsageRateMultiplier(body []byte) (float64, bool) {
	for _, path := range []string{
		"effective_rate_multiplier", "rate_multiplier", "data.effective_rate_multiplier",
		"data.rate_multiplier", "billing.effective_rate_multiplier", "data.billing.effective_rate_multiplier",
	} {
		value := gjson.GetBytes(body, path)
		if value.Exists() {
			multiplier := value.Float()
			if validSub2Multiplier(multiplier) && multiplier > 0 {
				return multiplier, true
			}
		}
	}
	return 0, false
}

func validSub2Multiplier(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
