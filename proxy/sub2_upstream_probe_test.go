package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codex2api/database"
)

func TestParseSub2EffectiveRateMultiplier(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want float64
	}{
		{name: "top level", body: `{"effective_rate_multiplier":1.25}`, want: 1.25},
		{name: "nested", body: `{"data":{"effective_rate_multiplier":2}}`, want: 2},
		{name: "fallback rate", body: `{"rate_multiplier":0.8}`, want: 0.8},
		{name: "zero rate", body: `{"rate_multiplier":0}`, want: 0},
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
	if _, ok := parseSub2EffectiveRateMultiplier([]byte(`{"billing":{"effective_rate_multiplier":2}}`)); ok {
		t.Fatal("unscoped billing payload must not be accepted as a usage fallback")
	}
	if _, ok := parseSub2EffectiveRateMultiplier([]byte(`{"effective_rate_multiplier":"2"}`)); ok {
		t.Fatal("string multiplier must not be accepted")
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
			name: "zero resolved rate",
			body: `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0,"resolved_rate_multiplier":0,"peak_rate_enabled":false,"effective_rate_multiplier":0,"observed_at":"2026-01-01T10:00:00Z"}`,
			want: 0,
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

func TestRefreshSub2UpstreamRateSingleflightAndKeepsLastSuccess(t *testing.T) {
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "probe.sqlite"))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	accountID, err := db.InsertAccountWithCredentials(ctx, "probe", map[string]interface{}{
		"sub2_upstream_rate_multiplier": 0.35,
	}, "")
	if err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.5,"resolved_rate_multiplier":0.5,"peak_rate_enabled":false,"effective_rate_multiplier":0.5,"observed_at":"2026-01-01T10:00:00Z"}`))
	}))
	defer server.Close()

	h := &Handler{db: db}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.refreshSub2UpstreamRate(accountID, server.URL, "key", "", nil)
		}()
	}
	deadline := time.After(2 * time.Second)
	for requests.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("probe did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	wg.Wait()
	if got := requests.Load(); got != 1 {
		t.Fatalf("concurrent refreshes = %d, want one upstream probe", got)
	}

	row, err := db.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got, ok := row.GetCredentialFloat64("sub2_upstream_rate_multiplier"); !ok || got != 0.5 {
		t.Fatalf("successful multiplier = %v/%v, want 0.5/true", got, ok)
	}

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance":10}`))
	})
	h.refreshSub2UpstreamRate(accountID, server.URL, "key", "", nil)
	row, err = db.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID after failure: %v", err)
	}
	if got, ok := row.GetCredentialFloat64("sub2_upstream_rate_multiplier"); !ok || got != 0.5 {
		t.Fatalf("failed probe changed last success = %v/%v, want 0.5/true", got, ok)
	}
}
