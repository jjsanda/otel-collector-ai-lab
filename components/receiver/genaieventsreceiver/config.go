package genaieventsreceiver

import (
	"errors"

	"go.opentelemetry.io/collector/config/confighttp"
)

// Config configures the genaievents receiver. It embeds the standard HTTP
// server config (endpoint, TLS, CORS, limits) and adds the ingest path.
type Config struct {
	confighttp.ServerConfig `mapstructure:",squash"`

	// Path is the HTTP route that accepts GenAI event payloads.
	Path string `mapstructure:"path"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.NetAddr.Endpoint == "" {
		return errors.New("endpoint cannot be empty")
	}
	if c.Path == "" {
		return errors.New("path cannot be empty")
	}
	return nil
}
