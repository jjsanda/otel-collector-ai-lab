package genairedactionprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

// typeStr is the component type used in collector configuration.
const typeStr = "genairedaction"

// NewFactory returns a factory for the genairedaction processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTraces, component.StabilityLevelBeta),
		processor.WithLogs(createLogs, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		RedactKeys:        genai.SensitiveContentKeys(),
		ScrubPatterns:     genai.DefaultScrubPatterns(),
		Mode:              ModeMask,
		MaskString:        "[REDACTED]",
		NormalizeProvider: true,
		ModelPricing:      genai.DefaultPricing(),
	}
}

func createTraces(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	rd, err := newRedactor(cfg.(*Config), set.Logger)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTraces(
		ctx, set, cfg, next,
		rd.processTraces,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}

func createLogs(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Logs,
) (processor.Logs, error) {
	rd, err := newRedactor(cfg.(*Config), set.Logger)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewLogs(
		ctx, set, cfg, next,
		rd.processLogs,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
