package genairedactionprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

func TestConfig_Validate(t *testing.T) {
	base := func() *Config { return createDefaultConfig().(*Config) }

	t.Run("default is valid", func(t *testing.T) {
		require.NoError(t, base().Validate())
	})
	t.Run("hash without salt is rejected", func(t *testing.T) {
		c := base()
		c.Mode = ModeHash
		c.Salt = ""
		require.Error(t, c.Validate())
	})
	t.Run("hash with salt is valid", func(t *testing.T) {
		c := base()
		c.Mode = ModeHash
		c.Salt = "pepper"
		require.NoError(t, c.Validate())
	})
	t.Run("unknown mode is rejected", func(t *testing.T) {
		c := base()
		c.Mode = "bogus"
		require.Error(t, c.Validate())
	})
	t.Run("invalid regex is rejected", func(t *testing.T) {
		c := base()
		c.ScrubPatterns = []string{"("}
		require.Error(t, c.Validate())
	})
	t.Run("negative pricing is rejected", func(t *testing.T) {
		c := base()
		c.ModelPricing = map[string]genai.ModelPrice{"x": {InputPer1K: -1}}
		require.Error(t, c.Validate())
	})
}

func TestFactory_Type(t *testing.T) {
	assert.Equal(t, "genairedaction", NewFactory().Type().String())
}
