package genairedactionprocessor

import (
	"fmt"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

// Mode controls how a sensitive attribute value is handled.
type Mode string

const (
	// ModeMask replaces the value with MaskString.
	ModeMask Mode = "mask"
	// ModeHash replaces the value with a salted SHA-256 digest (preserves
	// correlation without revealing content).
	ModeHash Mode = "hash"
	// ModeDelete removes the attribute entirely.
	ModeDelete Mode = "delete"
)

// Config configures the genairedaction processor.
type Config struct {
	// RedactKeys are attribute keys whose values are redacted (default: the
	// GenAI sensitive-content keys — prompts, responses, system instructions,
	// tool call args/results).
	RedactKeys []string `mapstructure:"redact_keys"`

	// ScrubPatterns are regexes applied to every remaining string attribute
	// value (and string log bodies); matches are masked. Defaults cover email,
	// card-like numbers, SSNs, and API keys.
	ScrubPatterns []string `mapstructure:"scrub_patterns"`

	// Mode selects how RedactKeys values are handled: mask | hash | delete.
	Mode Mode `mapstructure:"mode"`

	// MaskString is the replacement used by ModeMask and by the PII scrubber.
	MaskString string `mapstructure:"mask_string"`

	// Salt is mixed into the hash when Mode == hash. Required in that mode.
	Salt string `mapstructure:"salt"`

	// NormalizeProvider copies the deprecated gen_ai.system attribute onto
	// gen_ai.provider.name and removes the legacy key.
	NormalizeProvider bool `mapstructure:"normalize_provider"`

	// Environment, when set, is written to deployment.environment on every
	// resource that carries GenAI telemetry.
	Environment string `mapstructure:"environment"`

	// ModelPricing enables gen_ai.usage.cost_usd enrichment from token counts.
	ModelPricing map[string]genai.ModelPrice `mapstructure:"model_pricing"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeMask, ModeHash, ModeDelete:
	default:
		return fmt.Errorf("invalid mode %q (want one of mask|hash|delete)", c.Mode)
	}
	if c.Mode == ModeHash && c.Salt == "" {
		return fmt.Errorf("salt is required when mode=hash")
	}
	if _, err := genai.NewScrubber(c.ScrubPatterns, c.MaskString); err != nil {
		return fmt.Errorf("invalid scrub_patterns: %w", err)
	}
	for model, p := range c.ModelPricing {
		if p.InputPer1K < 0 || p.OutputPer1K < 0 {
			return fmt.Errorf("model_pricing[%q] has a negative price", model)
		}
	}
	return nil
}
