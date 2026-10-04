// Command api is the public entry point of the demo system. It receives "chat"
// requests and fans them out to the worker service, which in turn calls the
// db-adapter and llm-simulator. Every hop propagates W3C trace context, so one
// request produces a single distributed trace spanning all four services.
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

	"go.opentelemetry.io/otel"

	"github.com/jjsanda/otel-collector-ai-lab/services/shared"
)

func main() {
	ctx := context.Background()

	shutdown, err := shared.InitOTel(ctx, "api")
	if err != nil {
		slog.Error("failed to init OpenTelemetry", "error", err)
		os.Exit(1)
	}
	defer flush(shutdown)

	logger := shared.NewLogger("api")
	slog.SetDefault(logger)

	app := &server{
		logger:    logger,
		client:    shared.NewHTTPClient(),
		workerURL: shared.Env("WORKER_URL", "http://localhost:8081"),
		tracer:    otel.Tracer("api"),
	}
	if err := app.initMetrics(); err != nil {
		logger.Error("failed to init metrics", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("POST /chat", app.handleChat)
	mux.HandleFunc("GET /", app.handleDemo)

	addr := ":" + shared.Env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           shared.WrapHandler(mux, "api"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", addr, "worker_url", app.workerURL)
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
