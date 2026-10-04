package genaieventsreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

const (
	typeStr         = "genaievents"
	defaultEndpoint = "0.0.0.0:8099"
	defaultPath     = "/v1/genai-events"
)

// NewFactory returns a factory for the genaievents receiver.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		receiver.WithTraces(createTraces, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		ServerConfig: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  defaultEndpoint,
				Transport: confignet.TransportTypeTCP,
			},
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
		},
		Path: defaultPath,
	}
}

func createTraces(
	_ context.Context,
	set receiver.Settings,
	cfg component.Config,
	next consumer.Traces,
) (receiver.Traces, error) {
	return newReceiver(cfg.(*Config), set, next), nil
}
