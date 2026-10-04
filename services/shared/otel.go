// Package shared holds the common OpenTelemetry bootstrap and HTTP helpers used
// by the Go demo services (api, worker). It wires traces, metrics, and logs to
// an OTLP/HTTP endpoint configured entirely through the standard OTEL_* env
// vars, so the same binary works locally, in Compose, and on Kubernetes.
package shared

import (
	"context"
	"errors"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	otlplog "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otlpmetric "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otlptrace "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitOTel configures the global tracer, meter, and logger providers to export
// over OTLP/HTTP and installs the W3C tracecontext + baggage propagator. The
// returned function flushes and shuts everything down. Export configuration is
// read from OTEL_EXPORTER_OTLP_ENDPOINT / _PROTOCOL and related env vars.
func InitOTel(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		_ = os.Setenv("OTEL_SERVICE_NAME", serviceName)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(), // OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptrace.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetric.New(ctx)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(10*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logExp, err := otlplog.New(ctx)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx), lp.Shutdown(ctx))
	}, nil
}
