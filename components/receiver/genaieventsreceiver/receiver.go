// Package genaieventsreceiver accepts a bespoke "GenAI event" JSON payload over
// HTTP and converts it into OpenTelemetry trace spans that follow the gen_ai
// semantic conventions. It demonstrates a custom receiver lifecycle: an HTTP
// server built from confighttp, request decoding, and payload -> pdata mapping.
package genaieventsreceiver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	"github.com/jjsanda/otel-collector-ai-lab/components/internal/genai"
)

// maxBodyBytes caps the request body to avoid unbounded reads.
const maxBodyBytes = 1 << 20 // 1 MiB

// genaiEvent is the simplified ingestion payload.
type genaiEvent struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	Operation      string  `json:"operation"`
	ConversationID string  `json:"conversation_id"`
	Input          string  `json:"input"`
	Output         string  `json:"output"`
	InputTokens    int64   `json:"input_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	DurationMS     float64 `json:"duration_ms"`
	FinishReason   string  `json:"finish_reason"`
}

type genaiEventsReceiver struct {
	cfg      *Config
	settings receiver.Settings
	next     consumer.Traces

	server     *http.Server
	shutdownWG sync.WaitGroup
}

func newReceiver(cfg *Config, set receiver.Settings, next consumer.Traces) *genaiEventsReceiver {
	return &genaiEventsReceiver{cfg: cfg, settings: set, next: next}
}

func (r *genaiEventsReceiver) Start(ctx context.Context, host component.Host) error {
	mux := http.NewServeMux()
	mux.HandleFunc(r.cfg.Path, r.handle)

	srv, err := r.cfg.ToServer(ctx, host.GetExtensions(), r.settings.TelemetrySettings, mux)
	if err != nil {
		return err
	}
	r.server = srv

	ln, err := r.cfg.ToListener(ctx)
	if err != nil {
		return err
	}

	r.shutdownWG.Add(1)
	go func() {
		defer r.shutdownWG.Done()
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			r.settings.Logger.Error("genaievents server stopped", zap.Error(serveErr))
		}
	}()
	r.settings.Logger.Info("genaievents receiver started",
		zap.String("endpoint", r.cfg.NetAddr.Endpoint), zap.String("path", r.cfg.Path))
	return nil
}

func (r *genaiEventsReceiver) Shutdown(ctx context.Context) error {
	if r.server == nil {
		return nil
	}
	err := r.server.Shutdown(ctx)
	r.shutdownWG.Wait()
	return err
}

func (r *genaiEventsReceiver) handle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	var ev genaiEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if ev.Model == "" && ev.Operation == "" {
		http.Error(w, "event must include at least 'model' or 'operation'", http.StatusBadRequest)
		return
	}

	td := eventToTraces(ev)
	if err := r.next.ConsumeTraces(req.Context(), td); err != nil {
		http.Error(w, "pipeline error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// eventToTraces maps one GenAI event onto a single CLIENT span carrying the
// gen_ai.* attributes (including the sensitive prompt/response content, which a
// downstream redaction processor is expected to sanitize).
func eventToTraces(ev genaiEvent) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "genai-events")

	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	op := ev.Operation
	if op == "" {
		op = "chat"
	}
	name := op
	if ev.Model != "" {
		name = op + " " + ev.Model
	}
	span.SetName(name)
	span.SetKind(ptrace.SpanKindClient)
	span.SetTraceID(newTraceID())
	span.SetSpanID(newSpanID())

	end := time.Now()
	durMS := ev.DurationMS
	if math.IsNaN(durMS) || math.IsInf(durMS, 0) || durMS < 0 {
		durMS = 0
	}
	if durMS > 3_600_000 { // cap at 1 hour to keep timestamps sane
		durMS = 3_600_000
	}
	start := end.Add(-time.Duration(durMS * float64(time.Millisecond)))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))

	a := span.Attributes()
	putStr(a, genai.AttrOperationName, op)
	putStr(a, genai.AttrProviderName, ev.Provider)
	putStr(a, genai.AttrRequestModel, ev.Model)
	putStr(a, genai.AttrConversationID, ev.ConversationID)
	putStr(a, genai.AttrInputMessages, ev.Input)
	putStr(a, genai.AttrOutputMessages, ev.Output)
	if ev.InputTokens > 0 {
		a.PutInt(genai.AttrUsageInputTokens, ev.InputTokens)
	}
	if ev.OutputTokens > 0 {
		a.PutInt(genai.AttrUsageOutputTokens, ev.OutputTokens)
	}
	if ev.FinishReason != "" {
		a.PutEmptySlice(genai.AttrResponseFinishReasons).AppendEmpty().SetStr(ev.FinishReason)
	}
	return td
}

func putStr(a pcommon.Map, key, val string) {
	if val != "" {
		a.PutStr(key, val)
	}
}

func newTraceID() pcommon.TraceID {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return pcommon.TraceID(id)
}

func newSpanID() pcommon.SpanID {
	var id [8]byte
	_, _ = rand.Read(id[:])
	return pcommon.SpanID(id)
}
