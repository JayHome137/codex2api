package database

import "testing"

func TestCalculateSub2UpstreamCostKeepsSolHistoricalPrice(t *testing.T) {
	got, ok := CalculateSub2UpstreamCost(1_000, 1_000, 0, 0, 0, "gpt-5.6-sol", "", 1.5)
	if !ok {
		t.Fatal("expected supported Sol pricing")
	}
	if got.InputCost != .0075 || got.OutputCost != .045 || got.TotalCost != .0525 {
		t.Fatalf("cost = %#v, want input=.0075 output=.045 total=.0525", got)
	}
	if got.InputPricePerMToken != 7.5 {
		t.Fatalf("input price = %v, want 7.5", got.InputPricePerMToken)
	}
}

func TestCalculateSub2UpstreamCostUsesGPT6SolPrice(t *testing.T) {
	got, ok := CalculateSub2UpstreamCost(33_277, 177, 5_249, 0, 0, "gpt-6-sol", "", .07)
	if !ok {
		t.Fatal("expected GPT-6 Sol pricing")
	}
	assertFloatEqual(t, got.InputPricePerMToken, .14)
	assertFloatEqual(t, got.CacheReadPricePerMToken, .014)
	assertFloatEqual(t, got.OutputPricePerMToken, .7)
	assertFloatEqual(t, got.TotalCost, .004121306)
}

func TestCalculateSub2UpstreamCostUsesSameOpenAICacheWritePrice(t *testing.T) {
	got, ok := CalculateSub2UpstreamCost(100, 0, 0, 1_000, 1_000, "gpt-5.6-terra", "", 1)
	if !ok {
		t.Fatal("expected supported Terra pricing")
	}
	if got.CacheWrite5mPricePerMToken != 2.5 || got.CacheWrite1hPricePerMToken != 2.5 {
		t.Fatalf("cache write prices = %v/%v, want 2.5/2.5", got.CacheWrite5mPricePerMToken, got.CacheWrite1hPricePerMToken)
	}
	if got.CacheWrite5mCost != .0025 || got.CacheWrite1hCost != .0025 {
		t.Fatalf("cache write costs = %v/%v, want .0025/.0025", got.CacheWrite5mCost, got.CacheWrite1hCost)
	}
}

func TestCalculateSub2UpstreamCostLongContextIncludesCacheTokens(t *testing.T) {
	got, ok := CalculateSub2UpstreamCost(271_000, 1, 1_001, 0, 0, "gpt-6-astra", "", 1)
	if !ok || !got.LongContext {
		t.Fatalf("expected long context from complete context, got ok=%v result=%#v", ok, got)
	}
	if got.InputPricePerMToken != 20 || got.OutputPricePerMToken != 75 || got.CacheReadPricePerMToken != 2 {
		t.Fatalf("long prices = %v/%v/%v", got.InputPricePerMToken, got.OutputPricePerMToken, got.CacheReadPricePerMToken)
	}
}

func TestCalculateSub2UpstreamCostRequiresProbeMultiplier(t *testing.T) {
	if _, ok := CalculateSub2UpstreamCost(1, 1, 0, 0, 0, "gpt-5.6-luna", "", 0); ok {
		t.Fatal("probe failure must not be treated as 1x")
	}
}
