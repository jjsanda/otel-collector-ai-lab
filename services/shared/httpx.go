package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewHTTPClient returns an HTTP client whose transport injects W3C trace context
// into outbound requests and records a client span per call. Using this client
// for every downstream call is what keeps a trace connected across services.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// WrapHandler instruments an http.Handler as an OTel server handler. The route
// name is used as the span name prefix.
func WrapHandler(h http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(h, operation)
}

// HTTPError carries a non-2xx downstream response.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

// PostJSON marshals body, POSTs it to url with trace-context propagation, and
// decodes a 2xx response into out (out may be nil).
func PostJSON(ctx context.Context, client *http.Client, url string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return &HTTPError{Status: resp.StatusCode, Body: string(data)}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Env returns the value of key, or def when unset/empty.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
