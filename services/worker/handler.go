package main

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/errgroup"

	"github.com/jjsanda/otel-collector-ai-lab/services/shared"
)

type server struct {
	logger   *slog.Logger
	client   *http.Client
	dbURL    string
	llmURL   string
	requests metric.Int64Counter
}

func (s *server) initMetrics() error {
	c, err := otel.Meter("worker").Int64Counter(
		"worker.jobs",
		metric.WithDescription("Number of jobs processed by the worker."),
	)
	if err != nil {
		return err
	}
	s.requests = c
	return nil
}

type processRequest struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id"`
}

type processResponse struct {
	Answer       string `json:"answer"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Rows         int    `json:"rows"`
}

func (s *server) handleProcess(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	ctx := r.Context()
	s.requests.Add(ctx, 1)
	model := pickModel()

	// Fan out to the db-adapter and llm-simulator concurrently; both calls use
	// the request context so they are recorded as children of the same trace.
	var db dbResponse
	var llm llmResponse
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return shared.PostJSON(gctx, s.client, s.dbURL+"/query", dbRequest{
			Key:   req.ConversationID,
			Query: "SELECT * FROM conversation_history WHERE id = ?",
		}, &db)
	})
	g.Go(func() error {
		return shared.PostJSON(gctx, s.client, s.llmURL+"/generate", llmRequest{
			Prompt:         req.Prompt,
			Model:          model,
			ConversationID: req.ConversationID,
		}, &llm)
	})
	if err := g.Wait(); err != nil {
		s.logger.ErrorContext(ctx, "downstream call failed", "error", err)
		shared.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	s.logger.InfoContext(ctx, "job processed",
		"conversation_id", req.ConversationID, "model", llm.Model, "rows", db.Rows)

	shared.WriteJSON(w, http.StatusOK, processResponse{
		Answer:       llm.Text,
		Model:        llm.Model,
		InputTokens:  llm.InputTokens,
		OutputTokens: llm.OutputTokens,
		Rows:         db.Rows,
	})
}

type dbRequest struct {
	Key   string `json:"key"`
	Query string `json:"query"`
}

type dbResponse struct {
	Rows      int      `json:"rows"`
	Items     []string `json:"items"`
	LatencyMS float64  `json:"latency_ms"`
}

type llmRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	ConversationID string `json:"conversation_id"`
}

type llmResponse struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// pickModel varies the model so dashboards show a realistic mix.
func pickModel() string {
	models := []string{"gpt-4o-mini", "gpt-4o", "claude-3-5-sonnet", "claude-3-5-haiku", "gemini-1.5-pro"}
	return models[rand.Intn(len(models))]
}
