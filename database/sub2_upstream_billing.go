package database

import "strings"

// Sub2UpstreamCost is the cost charged by a Sub2API-style upstream account.
// It is deliberately separate from Codex2API's account/user billing fields.
type Sub2UpstreamCost struct {
	InputCost                  float64 `json:"input_cost"`
	OutputCost                 float64 `json:"output_cost"`
	CacheReadCost              float64 `json:"cache_read_cost"`
	CacheWrite5mCost           float64 `json:"cache_write_5m_cost"`
	CacheWrite1hCost           float64 `json:"cache_write_1h_cost"`
	TotalCost                  float64 `json:"total_cost"`
	InputPricePerMToken        float64 `json:"input_price_per_mtoken"`
	OutputPricePerMToken       float64 `json:"output_price_per_mtoken"`
	CacheReadPricePerMToken    float64 `json:"cache_read_price_per_mtoken"`
	CacheWrite5mPricePerMToken float64 `json:"cache_write_5m_price_per_mtoken"`
	CacheWrite1hPricePerMToken float64 `json:"cache_write_1h_price_per_mtoken"`
	RateMultiplier             float64 `json:"rate_multiplier"`
	LongContext                bool    `json:"long_context"`
	LongContextThreshold       int     `json:"long_context_threshold"`
}

type sub2UpstreamPrice struct {
	Input, Output, CacheRead, CacheWrite                 float64
	LongInput, LongOutput, LongCacheRead, LongCacheWrite float64
}

type sub2UpstreamModelPrice struct {
	Standard sub2UpstreamPrice
	Priority sub2UpstreamPrice
	Flex     sub2UpstreamPrice
}

// Prices intentionally retain the pre-adjustment Sol rates used by the current
// production Sub2API. Cache-write is one OpenAI price for both 5m and 1h.
var sub2UpstreamPrices = map[string]sub2UpstreamModelPrice{
	"gpt-5.4": {
		Standard: sub2UpstreamPrice{2.5, 15, .25, 2.5, 5, 22.5, .5, 5},
		Priority: sub2UpstreamPrice{5, 30, .5, 5, 10, 45, 1, 10},
		Flex:     sub2UpstreamPrice{1.25, 7.5, .125, 1.25, 2.5, 11.25, .25, 2.5},
	},
	"gpt-5.5": {
		Standard: sub2UpstreamPrice{5, 30, .5, 5, 10, 45, 1, 10},
		Priority: sub2UpstreamPrice{12.5, 75, 1.25, 12.5, 25, 112.5, 2.5, 25},
		Flex:     sub2UpstreamPrice{2.5, 15, .25, 2.5, 5, 22.5, .5, 5},
	},
	"gpt-5.5-pro": {
		Standard: sub2UpstreamPrice{30, 180, 3, 30, 60, 270, 6, 60},
		Priority: sub2UpstreamPrice{60, 360, 6, 60, 120, 540, 12, 120},
		Flex:     sub2UpstreamPrice{15, 90, 1.5, 15, 30, 135, 3, 30},
	},
	"gpt-5.6-sol": {
		Standard: sub2UpstreamPrice{5, 30, .5, 6.25, 10, 45, 1, 12.5},
		Priority: sub2UpstreamPrice{10, 60, 1, 12.5, 20, 90, 2, 25},
		Flex:     sub2UpstreamPrice{2.5, 15, .25, 3.125, 5, 22.5, .5, 6.25},
	},
	"gpt-5.6-terra": {
		Standard: sub2UpstreamPrice{2, 12, .2, 2.5, 4, 18, .4, 5},
		Priority: sub2UpstreamPrice{4, 24, .4, 5, 8, 36, .8, 10},
		Flex:     sub2UpstreamPrice{1, 6, .1, 1.25, 2, 9, .2, 2.5},
	},
	"gpt-5.6-luna": {
		Standard: sub2UpstreamPrice{.2, 1.2, .02, .25, .4, 1.8, .04, .5},
		Priority: sub2UpstreamPrice{.4, 2.4, .04, .5, .8, 3.6, .08, 1},
		Flex:     sub2UpstreamPrice{.1, .6, .01, .125, .2, .9, .02, .25},
	},
	"gpt-6-sol": {
		Standard: sub2UpstreamPrice{2, 10, .2, 2.5, 4, 15, .4, 5},
		Priority: sub2UpstreamPrice{4, 20, .4, 5, 8, 30, .8, 10},
		Flex:     sub2UpstreamPrice{1, 5, .1, 1.25, 2, 7.5, .2, 2.5},
	},
	"gpt-6-luna": {
		Standard: sub2UpstreamPrice{.1, .5, .01, .125, .2, .75, .02, .25},
		Priority: sub2UpstreamPrice{.2, 1, .02, .25, .4, 1.5, .04, .5},
		Flex:     sub2UpstreamPrice{.05, .25, .005, .0625, .1, .375, .01, .125},
	},
	"gpt-6-astra": {
		Standard: sub2UpstreamPrice{10, 50, 1, 12.5, 20, 75, 2, 25},
		Priority: sub2UpstreamPrice{20, 100, 2, 25, 40, 150, 4, 50},
		Flex:     sub2UpstreamPrice{5, 25, .5, 6.25, 10, 37.5, 1, 12.5},
	},
}

// CalculateSub2UpstreamCost applies Sub2API's context and TTL rules. The
// multiplier must come from a successful upstream probe; callers should not
// pass 1 as a fallback when the probe is unavailable.
func CalculateSub2UpstreamCost(inputTokens, outputTokens, cachedTokens, cacheWrite5mTokens, cacheWrite1hTokens int, model, serviceTier string, multiplier float64) (Sub2UpstreamCost, bool) {
	if multiplier <= 0 {
		return Sub2UpstreamCost{}, false
	}
	key := sub2UpstreamModelKey(model)
	prices, ok := sub2UpstreamPrices[key]
	if !ok {
		return Sub2UpstreamCost{}, false
	}
	price := prices.Standard
	tier := strings.ToLower(strings.TrimSpace(serviceTier))
	switch tier {
	case "fast", "priority", "ultrafast":
		price = prices.Priority
	case "flex":
		price = prices.Flex
	}

	inputTokens = maxInt(0, inputTokens)
	outputTokens = maxInt(0, outputTokens)
	cachedTokens = maxInt(0, cachedTokens)
	cacheWrite5mTokens = maxInt(0, cacheWrite5mTokens)
	cacheWrite1hTokens = maxInt(0, cacheWrite1hTokens)
	// Sub2API determines the context band from the complete input context,
	// including cache reads and cache creation tokens.
	contextTokens := inputTokens + cachedTokens + cacheWrite5mTokens + cacheWrite1hTokens
	long := contextTokens >= longContextThreshold
	if long && price.LongInput > 0 {
		price.Input, price.Output, price.CacheRead, price.CacheWrite = price.LongInput, price.LongOutput, price.LongCacheRead, price.LongCacheWrite
	}
	uncached := inputTokens - cachedTokens - cacheWrite5mTokens - cacheWrite1hTokens
	if uncached < 0 {
		uncached = 0
	}
	unit := func(tokens int, price float64) float64 { return float64(tokens) / 1_000_000 * price }
	result := Sub2UpstreamCost{
		InputCost: unit(uncached, price.Input), OutputCost: unit(outputTokens, price.Output),
		CacheReadCost: unit(cachedTokens, price.CacheRead), CacheWrite5mCost: unit(cacheWrite5mTokens, price.CacheWrite),
		CacheWrite1hCost: unit(cacheWrite1hTokens, price.CacheWrite), RateMultiplier: multiplier,
		InputPricePerMToken: price.Input * multiplier, OutputPricePerMToken: price.Output * multiplier,
		CacheReadPricePerMToken: price.CacheRead * multiplier, CacheWrite5mPricePerMToken: price.CacheWrite * multiplier,
		CacheWrite1hPricePerMToken: price.CacheWrite * multiplier, LongContext: long, LongContextThreshold: longContextThreshold,
	}
	result.InputCost *= multiplier
	result.OutputCost *= multiplier
	result.CacheReadCost *= multiplier
	result.CacheWrite5mCost *= multiplier
	result.CacheWrite1hCost *= multiplier
	result.TotalCost = result.InputCost + result.OutputCost + result.CacheReadCost + result.CacheWrite5mCost + result.CacheWrite1hCost
	return result, true
}

func sub2UpstreamModelKey(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(normalizeBillingModelName(model)))
	compact := strings.NewReplacer(" ", "-", "_", "-").Replace(normalized)
	switch {
	case strings.Contains(compact, "gpt-6-sol") || strings.Contains(compact, "gpt6-sol"):
		return "gpt-6-sol"
	case strings.Contains(compact, "gpt-6-terra") || strings.Contains(compact, "gpt6-terra"):
		return "gpt-6-terra"
	case strings.Contains(compact, "gpt-6-luna") || strings.Contains(compact, "gpt6-luna"):
		return "gpt-6-luna"
	case strings.Contains(compact, "gpt-6-astra") || strings.Contains(compact, "gpt6-astra"):
		return "gpt-6-astra"
	case strings.Contains(compact, "gpt-5.6-sol") || strings.Contains(compact, "gpt5-6-sol"):
		return "gpt-5.6-sol"
	case strings.Contains(compact, "gpt-5.6-terra") || strings.Contains(compact, "gpt5-6-terra"):
		return "gpt-5.6-terra"
	case strings.Contains(compact, "gpt-5.6-luna") || strings.Contains(compact, "gpt5-6-luna"):
		return "gpt-5.6-luna"
	}
	return CanonicalBillingModelKey(normalized)
}

func maxInt(v, floor int) int {
	if v < floor {
		return floor
	}
	return v
}
