package genaimetricsconnector

import (
	"fmt"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

// Config configures the genaimetrics connector.
type Config struct {
	// Namespace, when set, prefixes every emitted metric name (e.g. "lab.").
	Namespace string `mapstructure:"namespace"`

	// DurationBuckets are the explicit histogram bounds (seconds) for
	// gen_ai.client.operation.duration.
	DurationBuckets []float64 `mapstructure:"duration_buckets"`

	// TokenBuckets are the explicit histogram bounds (tokens) for
	// gen_ai.client.token.usage.
	TokenBuckets []float64 `mapstructure:"token_buckets"`

	// ModelPricing drives the gen_ai.client.cost.usd metric. When empty, the
	// cost metric is omitted.
	ModelPricing map[string]genai.ModelPrice `mapstructure:"model_pricing"`

	// MaxSeries caps the number of distinct (provider, model, operation) series
	// kept in memory, guarding against unbounded cardinality from untrusted
	// attribute values. New series beyond the cap are dropped. 0 disables the cap.
	MaxSeries int `mapstructure:"max_series"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if err := ascending("duration_buckets", c.DurationBuckets); err != nil {
		return err
	}
	if err := ascending("token_buckets", c.TokenBuckets); err != nil {
		return err
	}
	for model, p := range c.ModelPricing {
		if p.InputPer1K < 0 || p.OutputPer1K < 0 {
			return fmt.Errorf("model_pricing[%q] has a negative price", model)
		}
	}
	if c.MaxSeries < 0 {
		return fmt.Errorf("max_series must be >= 0")
	}
	return nil
}

func ascending(name string, bounds []float64) error {
	for i := 1; i < len(bounds); i++ {
		if bounds[i] <= bounds[i-1] {
			return fmt.Errorf("%s must be strictly ascending (got %v at index %d)", name, bounds[i], i)
		}
	}
	return nil
}
