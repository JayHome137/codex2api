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
