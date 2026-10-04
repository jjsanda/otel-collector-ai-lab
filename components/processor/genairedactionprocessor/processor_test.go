package genairedactionprocessor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

func strv(m pcommon.Map, k string) string {
	v, _ := m.Get(k)
	return v.AsString()
}

// genAISpanTraces builds a one-span trace carrying typical GenAI attributes.
func genAISpanTraces() ptrace.Traces {
	td := ptrace.NewTraces()
	a := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().Attributes()
	a.PutStr(genai.AttrSystem, "openai")
	a.PutStr(genai.AttrRequestModel, "gpt-4o-mini")
	a.PutStr(genai.AttrInputMessages, "my email is alice@example.com, card 4111 1111 1111 1111")
	a.PutStr(genai.AttrOutputMessages, "sure, here is your answer")
	a.PutInt(genai.AttrUsageInputTokens, 1000)
	a.PutInt(genai.AttrUsageOutputTokens, 2000)
	a.PutStr("user.note", "reach me at bob@example.com")
	return td
}

func spanAttrs(td ptrace.Traces) pcommon.Map {
	return td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
}

func TestProcessTraces_MaskAndEnrich(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, cfg.Validate())
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	out, err := rd.processTraces(context.Background(), genAISpanTraces())
	require.NoError(t, err)
	a := spanAttrs(out)

	// Provider normalized: gen_ai.system -> gen_ai.provider.name.
	assert.Equal(t, "openai", strv(a, genai.AttrProviderName))
	_, hasSystem := a.Get(genai.AttrSystem)
	assert.False(t, hasSystem, "deprecated gen_ai.system should be removed")

	// Sensitive content masked.
	assert.Equal(t, "[REDACTED]", strv(a, genai.AttrInputMessages))
	assert.Equal(t, "[REDACTED]", strv(a, genai.AttrOutputMessages))

	// PII scrubbed in a non-content attribute.
	assert.Equal(t, "reach me at [REDACTED]", strv(a, "user.note"))

	// Cost enriched: 1000/1000*0.00015 + 2000/1000*0.0006 = 0.00135.
	cost, ok := a.Get(genai.AttrUsageCostUSD)
	require.True(t, ok)
	assert.InDelta(t, 0.00135, cost.Double(), 1e-9)
}

func TestProcessTraces_Hash(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Mode = ModeHash
	cfg.Salt = "pepper"
	require.NoError(t, cfg.Validate())
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	out, err := rd.processTraces(context.Background(), genAISpanTraces())
	require.NoError(t, err)
	got := strv(spanAttrs(out), genai.AttrInputMessages)

	assert.True(t, strings.HasPrefix(got, "sha256:"), "want salted hash, got %q", got)
	assert.NotContains(t, got, "alice", "raw content must not survive")
}

func TestProcessTraces_Delete(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Mode = ModeDelete
	require.NoError(t, cfg.Validate())
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	out, err := rd.processTraces(context.Background(), genAISpanTraces())
	require.NoError(t, err)
	_, ok := spanAttrs(out).Get(genai.AttrInputMessages)
	assert.False(t, ok, "deleted key must be absent")
}

func TestProcessTraces_Environment(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Environment = "dev"
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	out, err := rd.processTraces(context.Background(), genAISpanTraces())
	require.NoError(t, err)
	res := out.ResourceSpans().At(0).Resource().Attributes()
	assert.Equal(t, "dev", strv(res, "deployment.environment"))
}

func TestProcessLogs_ScrubsBodyAndAttrs(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	ld := plog.NewLogs()
	lr := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	lr.Body().SetStr("user alice@example.com asked a question")
	lr.Attributes().PutStr(genai.AttrInputMessages, "secret prompt")

	out, err := rd.processLogs(context.Background(), ld)
	require.NoError(t, err)
	olr := out.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	assert.Equal(t, "user [REDACTED] asked a question", olr.Body().Str())
	assert.Equal(t, "[REDACTED]", strv(olr.Attributes(), genai.AttrInputMessages))
}

func TestProcessTraces_RedactsSpanEvents(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr(genai.AttrRequestModel, "gpt-4o-mini")
	ev := span.Events().AppendEmpty()
	ev.SetName("gen_ai.content.prompt")
	ev.Attributes().PutStr(genai.AttrInputMessages, "secret prompt, email a@b.com")
	ev.Attributes().PutStr("note", "call me at x@y.com")

	out, err := rd.processTraces(context.Background(), td)
	require.NoError(t, err)
	oev := out.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events().At(0)

	assert.Equal(t, "[REDACTED]", strv(oev.Attributes(), genai.AttrInputMessages))
	assert.Equal(t, "call me at [REDACTED]", strv(oev.Attributes(), "note"))
}

func TestProcessLogs_StructuredBody(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	rd, err := newRedactor(cfg, nil)
	require.NoError(t, err)

	ld := plog.NewLogs()
	lr := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	body := lr.Body().SetEmptyMap()
	body.PutStr(genai.AttrInputMessages, "my secret prompt")
	body.PutStr("free", "reach me at bob@example.com")

	out, err := rd.processLogs(context.Background(), ld)
	require.NoError(t, err)
	ob := out.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Map()

	assert.Equal(t, "[REDACTED]", strv(ob, genai.AttrInputMessages))
	assert.Equal(t, "reach me at [REDACTED]", strv(ob, "free"))
}
