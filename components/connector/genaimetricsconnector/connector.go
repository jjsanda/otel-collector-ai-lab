// Package genaimetricsconnector is an OpenTelemetry Collector connector that
// derives GenAI metrics from trace spans. For each span carrying gen_ai.*
// attributes it maintains cumulative series — operation count, operation
// duration, token usage (split by input/output), and estimated cost — keyed by
// provider, model, and operation, and emits them as OTLP metrics.
package genaimetricsconnector

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

const scopeName = "github.com/jjsanda/otel-collector-ai-lab/components/connector/genaimetrics"

// dims is the label set shared by every series.
type dims struct {
	provider  string
	model     string
	operation string
}

// tokenDims adds the token type (input/output) for the token-usage histogram.
type tokenDims struct {
	dims
	tokenType string
}

type genaiMetricsConnector struct {
	component.StartFunc
	component.ShutdownFunc

	cfg    *Config
	next   consumer.Metrics
	logger *zap.Logger

	mu         sync.Mutex
	startTime  time.Time
	requests   map[dims]int64
	costs      map[dims]float64
	durations  map[dims]*histogram
	tokens     map[tokenDims]*histogram
	dropped    uint64
	warnedDrop bool
}

func newConnector(cfg *Config, set connector.Settings, next consumer.Metrics) *genaiMetricsConnector {
	logger := set.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &genaiMetricsConnector{
		cfg:       cfg,
		next:      next,
		logger:    logger,
		startTime: time.Now(),
		requests:  map[dims]int64{},
		costs:     map[dims]float64{},
		durations: map[dims]*histogram{},
		tokens:    map[tokenDims]*histogram{},
	}
}

func (c *genaiMetricsConnector) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeTraces updates the cumulative state from the batch and forwards a
// snapshot of all series to the next metrics consumer.
func (c *genaiMetricsConnector) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	c.mu.Lock()
	c.aggregate(td)
	m := c.buildMetrics()
	c.mu.Unlock()

	if m.DataPointCount() == 0 {
		return nil
	}
	return c.next.ConsumeMetrics(ctx, m)
}

func (c *genaiMetricsConnector) aggregate(td ptrace.Traces) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		sss := rss.At(i).ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				c.aggregateSpan(spans.At(k))
			}
		}
	}
}

func (c *genaiMetricsConnector) aggregateSpan(span ptrace.Span) {
	attrs := span.Attributes()
	if !genai.IsGenAIAttrs(attrs) {
		return
	}
	d := dims{
		provider:  providerOf(attrs),
		model:     genai.StrAttr(attrs, genai.AttrRequestModel),
		operation: genai.StrAttr(attrs, genai.AttrOperationName),
	}

	if c.atCapacity(d) {
		return
	}

	c.requests[d]++

	dur := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Seconds()
	if dur < 0 {
		dur = 0
	}
	c.durationHistogram(d).observe(dur)

	in := genai.IntAttr(attrs, genai.AttrUsageInputTokens)
	out := genai.IntAttr(attrs, genai.AttrUsageOutputTokens)
	if in > 0 {
		c.tokenHistogram(d, genai.TokenTypeInput).observe(float64(in))
	}
	if out > 0 {
		c.tokenHistogram(d, genai.TokenTypeOutput).observe(float64(out))
	}

	if len(c.cfg.ModelPricing) > 0 && d.model != "" {
		if cost, ok := genai.EstimateCostUSD(c.cfg.ModelPricing, d.model, in, out); ok {
			c.costs[d] += cost
		}
	}
}

// atCapacity reports whether adding a new series would exceed MaxSeries.
// Existing series are always allowed to keep accumulating.
func (c *genaiMetricsConnector) atCapacity(d dims) bool {
	if c.cfg.MaxSeries <= 0 {
		return false
	}
	if _, exists := c.requests[d]; exists {
		return false
	}
	if len(c.requests) < c.cfg.MaxSeries {
		return false
	}
	c.dropped++
	if !c.warnedDrop {
		c.warnedDrop = true
		c.logger.Warn("series cardinality cap reached; dropping new GenAI series",
			zap.Int("max_series", c.cfg.MaxSeries))
	}
	return true
}

func (c *genaiMetricsConnector) durationHistogram(d dims) *histogram {
	h, ok := c.durations[d]
	if !ok {
		h = newHistogram(c.cfg.DurationBuckets)
		c.durations[d] = h
	}
	return h
}

func (c *genaiMetricsConnector) tokenHistogram(d dims, tokenType string) *histogram {
	k := tokenDims{dims: d, tokenType: tokenType}
	h, ok := c.tokens[k]
	if !ok {
		h = newHistogram(c.cfg.TokenBuckets)
		c.tokens[k] = h
	}
	return h
}

func providerOf(attrs pcommon.Map) string {
	if p := genai.StrAttr(attrs, genai.AttrProviderName); p != "" {
		return p
	}
	return genai.StrAttr(attrs, genai.AttrSystem)
}

func (c *genaiMetricsConnector) buildMetrics() pmetric.Metrics {
	m := pmetric.NewMetrics()
	sm := m.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(scopeName)
	start := pcommon.NewTimestampFromTime(c.startTime)
	now := pcommon.NewTimestampFromTime(time.Now())

	if len(c.requests) > 0 {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(c.name("gen_ai.client.operation.count"))
		metric.SetDescription("Number of GenAI operations observed.")
		metric.SetUnit("{operation}")
		sum := metric.SetEmptySum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for d, v := range c.requests {
			dp := sum.DataPoints().AppendEmpty()
			dp.SetStartTimestamp(start)
			dp.SetTimestamp(now)
			dp.SetIntValue(v)
			setDims(dp.Attributes(), d)
		}
	}

	if len(c.durations) > 0 {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(c.name("gen_ai.client.operation.duration"))
		metric.SetDescription("Duration of GenAI operations.")
		metric.SetUnit("s")
		h := metric.SetEmptyHistogram()
		h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for d, hist := range c.durations {
			dp := h.DataPoints().AppendEmpty()
			dp.SetStartTimestamp(start)
			dp.SetTimestamp(now)
			hist.writeTo(dp)
			setDims(dp.Attributes(), d)
		}
	}

	if len(c.tokens) > 0 {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(c.name("gen_ai.client.token.usage"))
		metric.SetDescription("Number of tokens used, by token type.")
		metric.SetUnit("{token}")
		h := metric.SetEmptyHistogram()
		h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for k, hist := range c.tokens {
			dp := h.DataPoints().AppendEmpty()
			dp.SetStartTimestamp(start)
			dp.SetTimestamp(now)
			hist.writeTo(dp)
			setDims(dp.Attributes(), k.dims)
			dp.Attributes().PutStr(genai.AttrTokenType, k.tokenType)
		}
	}

	if len(c.costs) > 0 {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(c.name("gen_ai.client.cost.usd"))
		metric.SetDescription("Estimated GenAI cost in USD, derived from token usage.")
		metric.SetUnit("{usd}")
		sum := metric.SetEmptySum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for d, v := range c.costs {
			dp := sum.DataPoints().AppendEmpty()
			dp.SetStartTimestamp(start)
			dp.SetTimestamp(now)
			dp.SetDoubleValue(v)
			setDims(dp.Attributes(), d)
		}
	}

	return m
}

func (c *genaiMetricsConnector) name(base string) string {
	return c.cfg.Namespace + base
}

func setDims(attrs pcommon.Map, d dims) {
	if d.provider != "" {
		attrs.PutStr(genai.AttrProviderName, d.provider)
	}
	if d.model != "" {
		attrs.PutStr(genai.AttrRequestModel, d.model)
	}
	if d.operation != "" {
		attrs.PutStr(genai.AttrOperationName, d.operation)
	}
}
