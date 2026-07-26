package api

// modelRate is the price per 1M tokens, split by prompt vs completion —
// providers charge them at different rates (completion is usually dearer).
type modelRate struct {
	promptPerM     float64
	completionPerM float64
}

// rate table — prices per 1M tokens, seeded for the models we support.
var modelRates = map[string]modelRate{
	"gpt-4o-mini": {promptPerM: 0.15, completionPerM: 0.60},
	// add more models here as needed
}

// costUSD converts a response's token usage into a dollar cost.
// Returns ok=false if the model isn't in the table, so the caller can notice a
// config mistake instead of silently charging 0.
func costUSD(model string, promptTokens, completionTokens int) (float64, bool) {
	rate, ok := modelRates[model]
	if !ok {
		return 0, false
	}

	cost := 0.0
	cost += rate.promptPerM * float64(promptTokens) / 1_000_000
	cost += rate.completionPerM * float64(completionTokens) / 1_000_000
	return cost, true

}
