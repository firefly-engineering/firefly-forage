package telemetry

import (
	"context"
	"os"

	"github.com/honeycombio/otel-config-go/otelconfig"
)

// Init sets up OpenTelemetry tracing via standard OTEL_* env vars.
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, telemetry is disabled (no-op).
// Returns a shutdown function that must be called on exit.
//
// Configuration is done entirely via environment variables:
//
//	OTEL_SERVICE_NAME            - service name (fallback: serviceName param)
//	OTEL_EXPORTER_OTLP_ENDPOINT - e.g. https://api.honeycomb.io:443
//	OTEL_EXPORTER_OTLP_HEADERS  - e.g. x-honeycomb-team=YOUR_KEY
func Init(ctx context.Context, serviceName string) (shutdown func(), err error) {
	noop := func() {}

	// Only initialize when an exporter endpoint is configured.
	// otel-config-go defaults to localhost:4317 which would cause
	// connection noise when telemetry is not desired.
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return noop, nil
	}

	shutdown, err = otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(serviceName),
	)
	if err != nil {
		return noop, err
	}
	return shutdown, nil
}
