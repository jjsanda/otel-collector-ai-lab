package genai

// ModelPrice captures illustrative USD pricing per 1000 tokens for a model.
type ModelPrice struct {
	InputPer1K  float64 `mapstructure:"input_per_1k"`
	OutputPer1K float64 `mapstructure:"output_per_1k"`
}

// DefaultPricing returns a small built-in price table (USD per 1K tokens).
// Values are illustrative for the lab and are NOT authoritative; override via
// the processor/connector `model_pricing` config in production.
func DefaultPricing() map[string]ModelPrice {
	return map[string]ModelPrice{
		"gpt-4o":            {InputPer1K: 0.0025, OutputPer1K: 0.0100},
		"gpt-4o-mini":       {InputPer1K: 0.00015, OutputPer1K: 0.00060},
		"claude-3-5-sonnet": {InputPer1K: 0.0030, OutputPer1K: 0.0150},
		"claude-3-5-haiku":  {InputPer1K: 0.0008, OutputPer1K: 0.0040},
		"gemini-1.5-pro":    {InputPer1K: 0.00125, OutputPer1K: 0.0050},
		"llama-3.1-8b":      {InputPer1K: 0.0, OutputPer1K: 0.0},
	}
}

// EstimateCostUSD returns the estimated request cost given token counts. The
// boolean is false when no pricing is known for the model.
func EstimateCostUSD(pricing map[string]ModelPrice, model string, inputTokens, outputTokens int64) (float64, bool) {
	p, ok := pricing[model]
	if !ok {
		return 0, false
	}
	cost := float64(inputTokens)/1000.0*p.InputPer1K + float64(outputTokens)/1000.0*p.OutputPer1K
	return cost, true
}
