package genaimetricsconnector

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

const typeStr = "genaimetrics"

// NewFactory returns a factory for the genaimetrics connector.
func NewFactory() connector.Factory {
	return connector.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		connector.WithTracesToMetrics(createTracesToMetrics, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		DurationBuckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		TokenBuckets:    []float64{10, 50, 100, 250, 500, 1000, 2000, 4000, 8000, 16000},
		ModelPricing:    genai.DefaultPricing(),
		MaxSeries:       2000,
	}
}

func createTracesToMetrics(
	_ context.Context,
	set connector.Settings,
	cfg component.Config,
	next consumer.Metrics,
) (connector.Traces, error) {
	return newConnector(cfg.(*Config), set, next), nil
}
