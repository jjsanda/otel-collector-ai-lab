package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/jjsanda/otel-collector-ai-lab/services/shared"
)

type server struct {
	logger    *slog.Logger
	client    *http.Client
	workerURL string
	tracer    trace.Tracer
	requests  metric.Int64Counter
}

func (s *server) initMetrics() error {
	c, err := otel.Meter("api").Int64Counter(
		"api.requests",
		metric.WithDescription("Number of chat requests handled by the API."),
	)
	if err != nil {
		return err
	}
	s.requests = c
	return nil
}

type chatRequest struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id"`
}

type chatResponse struct {
	Answer  string `json:"answer"`
	Model   string `json:"model"`
	Tokens  int64  `json:"tokens"`
	Rows    int    `json:"rows"`
	TraceID string `json:"trace_id"`
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	s.process(w, r, req)
}

// handleDemo generates a sample request (with deliberately PII-laden prompts) so
// the system can be exercised with a simple GET and redaction is demonstrable.
func (s *server) handleDemo(w http.ResponseWriter, r *http.Request) {
	s.process(w, r, chatRequest{
		Prompt:         samplePrompt(),
		ConversationID: fmt.Sprintf("demo-%05d", rand.Intn(100000)),
	})
}

func (s *server) process(w http.ResponseWriter, r *http.Request, req chatRequest) {
	ctx := r.Context()
	s.requests.Add(ctx, 1)

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("app.conversation_id", req.ConversationID))

	var wr workerResponse
	if err := shared.PostJSON(ctx, s.client, s.workerURL+"/process", workerRequest{
		Prompt:         req.Prompt,
		ConversationID: req.ConversationID,
	}, &wr); err != nil {
		s.logger.ErrorContext(ctx, "worker call failed", "error", err)
		shared.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	s.logger.InfoContext(ctx, "chat handled",
		"conversation_id", req.ConversationID, "model", wr.Model, "rows", wr.Rows)

	shared.WriteJSON(w, http.StatusOK, chatResponse{
		Answer:  wr.Answer,
		Model:   wr.Model,
		Tokens:  wr.InputTokens + wr.OutputTokens,
		Rows:    wr.Rows,
		TraceID: span.SpanContext().TraceID().String(),
	})
}

type workerRequest struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id"`
}

type workerResponse struct {
	Answer       string `json:"answer"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Rows         int    `json:"rows"`
}

// samplePrompt returns a prompt containing realistic-looking PII so the
// collector's redaction processor has something meaningful to sanitize.
func samplePrompt() string {
	prompts := []string{
		"Summarize my last invoice. My email is alice@example.com and card 4111 1111 1111 1111.",
		"Draft a reply to bob@example.com about the SSN 123-45-6789 we have on file.",
		"Plan my trip and email the itinerary to carol@example.org.",
	}
	return prompts[rand.Intn(len(prompts))]
}
