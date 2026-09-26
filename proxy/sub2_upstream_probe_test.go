package proxy

import "testing"

func TestParseSub2EffectiveRateMultiplier(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want float64
	}{
		{name: "top level", body: `{"effective_rate_multiplier":1.25}`, want: 1.25},
		{name: "nested", body: `{"data":{"billing":{"effective_rate_multiplier":2}}}`, want: 2},
		{name: "fallback rate", body: `{"rate_multiplier":0.8}`, want: 0.8},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseSub2EffectiveRateMultiplier([]byte(test.body))
			if !ok || got != test.want {
				t.Fatalf("got %v/%v, want %v/true", got, ok, test.want)
			}
		})
	}
	if _, ok := parseSub2EffectiveRateMultiplier([]byte(`{"balance":10}`)); ok {
		t.Fatal("balance alone must not imply an effective multiplier")
	}
}

func TestParseSub2BillingRateMultiplierRequiresCompatibleProtocol(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want float64
	}{
		{
			name: "resolved rate",
			body: `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
			want: 0.35,
		},
		{
			name: "effective rate overrides resolved rate",
			body: `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.5,"resolved_rate_multiplier":0.5,"peak_rate_enabled":true,"peak_start":"09:00","peak_end":"11:00","peak_rate_multiplier":0.7,"applied_peak_multiplier":0.7,"effective_rate_multiplier":0.35,"timezone":"UTC","observed_at":"2026-01-01T10:00:00Z"}`,
			want: 0.35,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseSub2BillingRateMultiplier([]byte(test.body))
			if !ok || got != test.want {
				t.Fatalf("got %v/%v, want %v/true", got, ok, test.want)
			}
		})
	}
	for _, body := range []string{
		`{"object":"other.billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":2,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"request","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.5,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.5,"observed_at":"2026-01-01T10:00:00Z"}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":false,"effective_rate_multiplier":0.35}`,
		`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.35,"resolved_rate_multiplier":0.35,"peak_rate_enabled":true,"effective_rate_multiplier":0.35,"observed_at":"2026-01-01T10:00:00Z"}`,
	} {
		if _, ok := parseSub2BillingRateMultiplier([]byte(body)); ok {
			t.Fatalf("invalid billing payload was accepted: %s", body)
		}
	}
}
