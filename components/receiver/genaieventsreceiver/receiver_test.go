package genaieventsreceiver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

func testSettings() receiver.Settings {
	return receiver.Settings{
		ID:                component.NewID(component.MustNewType(typeStr)),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		BuildInfo:         component.NewDefaultBuildInfo(),
	}
}

func strv(m pcommon.Map, k string) string {
	v, _ := m.Get(k)
	return v.AsString()
}

func TestHandle_AcceptsEvent(t *testing.T) {
	sink := new(consumertest.TracesSink)
	r := newReceiver(createDefaultConfig().(*Config), testSettings(), sink)

	body := `{"provider":"openai","model":"gpt-4o-mini","operation":"chat",` +
		`"input":"hi there","output":"hello!","input_tokens":12,"output_tokens":8,` +
		`"duration_ms":250,"finish_reason":"stop","conversation_id":"c-1"}`
	w := httptest.NewRecorder()
	r.handle(w, httptest.NewRequest(http.MethodPost, "/v1/genai-events", strings.NewReader(body)))

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Len(t, sink.AllTraces(), 1)

	span := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, "chat gpt-4o-mini", span.Name())
	assert.Equal(t, ptrace.SpanKindClient, span.Kind())

	a := span.Attributes()
	assert.Equal(t, "openai", strv(a, genai.AttrProviderName))
	assert.Equal(t, "gpt-4o-mini", strv(a, genai.AttrRequestModel))
	assert.Equal(t, "chat", strv(a, genai.AttrOperationName))
	assert.Equal(t, "hi there", strv(a, genai.AttrInputMessages))
	assert.Equal(t, "hello!", strv(a, genai.AttrOutputMessages))
	assert.Equal(t, "c-1", strv(a, genai.AttrConversationID))
	in, _ := a.Get(genai.AttrUsageInputTokens)
	assert.EqualValues(t, 12, in.Int())
}

func TestHandle_Rejects(t *testing.T) {
	sink := new(consumertest.TracesSink)
	r := newReceiver(createDefaultConfig().(*Config), testSettings(), sink)

	cases := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"wrong method", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"bad json", http.MethodPost, "{not json", http.StatusBadRequest},
		{"missing model and operation", http.MethodPost, "{}", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.handle(w, httptest.NewRequest(tc.method, "/v1/genai-events", strings.NewReader(tc.body)))
			assert.Equal(t, tc.want, w.Code)
		})
	}
	assert.Empty(t, sink.AllTraces(), "rejected requests must not emit spans")
}

func TestStartShutdown(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.NetAddr.Endpoint = "127.0.0.1:0" // ephemeral port
	r := newReceiver(cfg, testSettings(), new(consumertest.TracesSink))

	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, r.Shutdown(context.Background()))
}

func TestFactory_Type(t *testing.T) {
	assert.Equal(t, "genaievents", NewFactory().Type().String())
}
