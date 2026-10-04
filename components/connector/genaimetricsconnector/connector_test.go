package genaimetricsconnector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

func testSettings() connector.Settings {
	return connector.Settings{
		ID:                component.NewID(component.MustNewType(typeStr)),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		BuildInfo:         component.NewDefaultBuildInfo(),
	}
}

func addGenAISpan(td ptrace.Traces, provider, model, op string, inTok, outTok int64, dur time.Duration) {
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	a := span.Attributes()
	a.PutStr(genai.AttrProviderName, provider)
	a.PutStr(genai.AttrRequestModel, model)
	a.PutStr(genai.AttrOperationName, op)
	a.PutInt(genai.AttrUsageInputTokens, inTok)
	a.PutInt(genai.AttrUsageOutputTokens, outTok)
	end := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(end.Add(-dur)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
}

func collect(m pmetric.Metrics) map[string]pmetric.Metric {
	out := map[string]pmetric.Metric{}
	rms := m.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		sms := rms.At(i).ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			ms := sms.At(j).Metrics()
			for k := 0; k < ms.Len(); k++ {
				out[ms.At(k).Name()] = ms.At(k)
			}
		}
	}
	return out
}

func TestConnector_DerivesMetrics(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, cfg.Validate())
	sink := new(consumertest.MetricsSink)
	conn := newConnector(cfg, testSettings(), sink)

	td := ptrace.NewTraces()
	addGenAISpan(td, "openai", "gpt-4o-mini", "chat", 100, 200, 500*time.Millisecond)
	addGenAISpan(td, "openai", "gpt-4o-mini", "chat", 100, 200, 700*time.Millisecond)
	// A non-GenAI span must be ignored.
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("db.query")

	require.NoError(t, conn.ConsumeTraces(context.Background(), td))
	require.Len(t, sink.AllMetrics(), 1)
	metrics := collect(sink.AllMetrics()[0])

	count := metrics["gen_ai.client.operation.count"]
	require.NotEqual(t, pmetric.MetricTypeEmpty, count.Type())
	assert.EqualValues(t, 2, count.Sum().DataPoints().At(0).IntValue())

	tokens := metrics["gen_ai.client.token.usage"]
	require.Equal(t, 2, tokens.Histogram().DataPoints().Len(), "one input + one output series")
	var totalObs uint64
	for i := 0; i < tokens.Histogram().DataPoints().Len(); i++ {
		totalObs += tokens.Histogram().DataPoints().At(i).Count()
	}
	assert.EqualValues(t, 4, totalObs, "two spans contribute input and output each")

	dur := metrics["gen_ai.client.operation.duration"]
	assert.InDelta(t, 1.2, dur.Histogram().DataPoints().At(0).Sum(), 0.3)

	cost := metrics["gen_ai.client.cost.usd"]
	assert.Greater(t, cost.Sum().DataPoints().At(0).DoubleValue(), 0.0)
}

func TestConnector_Cumulative(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := new(consumertest.MetricsSink)
	conn := newConnector(cfg, testSettings(), sink)

	for i := 0; i < 3; i++ {
		td := ptrace.NewTraces()
		addGenAISpan(td, "openai", "gpt-4o-mini", "chat", 10, 10, time.Second)
		require.NoError(t, conn.ConsumeTraces(context.Background(), td))
	}
	all := sink.AllMetrics()
	last := collect(all[len(all)-1])
	assert.EqualValues(t, 3, last["gen_ai.client.operation.count"].Sum().DataPoints().At(0).IntValue())
}

func TestConnector_IgnoresNonGenAI(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	sink := new(consumertest.MetricsSink)
	conn := newConnector(cfg, testSettings(), sink)

	td := ptrace.NewTraces()
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("db.query")
	require.NoError(t, conn.ConsumeTraces(context.Background(), td))
	assert.Empty(t, sink.AllMetrics())
}

func TestHistogram_BucketBoundaries(t *testing.T) {
	h := newHistogram([]float64{0.05, 0.1})
	h.observe(0.05)      // <= 0.05  -> bucket 0
	h.observe(0.0500001) // (0.05, 0.1] -> bucket 1
	h.observe(1.0)       // > 0.1    -> bucket 2 (overflow)

	assert.EqualValues(t, 3, h.count)
	assert.InDelta(t, 1.1000001, h.sum, 1e-9)
	assert.Equal(t, []uint64{1, 1, 1}, h.buckets)
}

func TestConnector_ReadOnly(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conn := newConnector(cfg, testSettings(), new(consumertest.MetricsSink))
	assert.False(t, conn.Capabilities().MutatesData)

	td := ptrace.NewTraces()
	addGenAISpan(td, "openai", "gpt-4o-mini", "chat", 100, 200, time.Second)
	require.NoError(t, conn.ConsumeTraces(context.Background(), td))

	// The input traces must be untouched (MutatesData == false).
	a := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	v, _ := a.Get(genai.AttrUsageInputTokens)
	assert.EqualValues(t, 100, v.Int())
}

func TestConnector_MaxSeriesCap(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.MaxSeries = 2
	sink := new(consumertest.MetricsSink)
	conn := newConnector(cfg, testSettings(), sink)

	td := ptrace.NewTraces()
	addGenAISpan(td, "openai", "m1", "chat", 1, 1, time.Second)
	addGenAISpan(td, "openai", "m2", "chat", 1, 1, time.Second)
	addGenAISpan(td, "openai", "m3", "chat", 1, 1, time.Second) // dropped by the cap

	require.NoError(t, conn.ConsumeTraces(context.Background(), td))
	metrics := collect(sink.AllMetrics()[0])
	assert.Equal(t, 2, metrics["gen_ai.client.operation.count"].Sum().DataPoints().Len())
}

func TestFactory_Type(t *testing.T) {
	assert.Equal(t, "genaimetrics", NewFactory().Type().String())
}
