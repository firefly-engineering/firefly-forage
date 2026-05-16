package telemetry

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// otelEnvVars are the OTEL_* environment variables to forward to child
// processes so they can export to the same backend.
var otelEnvVars = []string{
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_HEADERS",
	"OTEL_EXPORTER_OTLP_PROTOCOL",
}

// EnvPrefix returns a shell-compatible environment variable prefix that
// propagates W3C trace context (TRACEPARENT) and OTLP export configuration
// to child processes invoked via shell commands.
//
// Returns "" when no active span exists or OTEL is not configured.
//
// Example output: TRACEPARENT=00-abc...-def...-01 OTEL_EXPORTER_OTLP_ENDPOINT='https://api.honeycomb.io:443' cmd
func EnvPrefix(ctx context.Context) string {
	var parts []string

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if tp := carrier.Get("traceparent"); tp != "" {
		parts = append(parts, "TRACEPARENT="+tp)
	}

	for _, key := range otelEnvVars {
		if val := os.Getenv(key); val != "" {
			parts = append(parts, key+"="+shellQuote(val))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " "
}

// PropagationEnv returns key=value pairs suitable for appending to
// exec.Cmd.Env. Includes TRACEPARENT and any set OTEL_* vars.
func PropagationEnv(ctx context.Context) []string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	var env []string
	if tp := carrier.Get("traceparent"); tp != "" {
		env = append(env, "TRACEPARENT="+tp)
	}

	for _, key := range otelEnvVars {
		if val := os.Getenv(key); val != "" {
			env = append(env, key+"="+val)
		}
	}
	return env
}

// ContextFromEnv extracts W3C trace context from the TRACEPARENT
// environment variable. Returns the input context unchanged if
// TRACEPARENT is not set.
func ContextFromEnv(ctx context.Context) context.Context {
	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{}
	carrier.Set("traceparent", tp)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
