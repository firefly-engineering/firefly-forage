package telemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "forage-ctl"

// Start creates a span from the package-level tracer.
func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, opts...)
}

// Command creates a span for a CLI command invocation.
func Command(ctx context.Context, command string) (context.Context, trace.Span) {
	return Start(ctx, "cmd."+command, trace.WithAttributes(
		attribute.String("command", command),
	))
}

// Exec creates a span for a subprocess execution.
func Exec(ctx context.Context, binary string, args ...string) (context.Context, trace.Span) {
	return Start(ctx, "exec."+binary, trace.WithAttributes(
		attribute.String("binary", binary),
		attribute.String("args", strings.Join(args, " ")),
	))
}

// RecordError records an error on the current span and sets error status.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
