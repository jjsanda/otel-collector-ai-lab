// Command worker receives work from the api service and fans out to the
// db-adapter and llm-simulator services concurrently, propagating trace context
// to both so they join the same distributed trace.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jjsanda/otel-collector-ai-lab/services/shared"
)

func main() {
	ctx := context.Background()

	shutdown, err := shared.InitOTel(ctx, "worker")
	if err != nil {
		slog.Error("failed to init OpenTelemetry", "error", err)
		os.Exit(1)
	}
	defer flush(shutdown)

	logger := shared.NewLogger("worker")
	slog.SetDefault(logger)

	app := &server{
		logger: logger,
		client: shared.NewHTTPClient(),
		dbURL:  shared.Env("DB_ADAPTER_URL", "http://localhost:8082"),
		llmURL: shared.Env("LLM_SIMULATOR_URL", "http://localhost:8083"),
	}
	if err := app.initMetrics(); err != nil {
		logger.Error("failed to init metrics", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("POST /process", app.handleProcess)

	addr := ":" + shared.Env("PORT", "8081")
	srv := &http.Server{
		Addr:              addr,
		Handler:           shared.WrapHandler(mux, "worker"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("worker listening", "addr", addr, "db_url", app.dbURL, "llm_url", app.llmURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
}

func flush(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}

func health(w http.ResponseWriter, _ *http.Request) {
	shared.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
