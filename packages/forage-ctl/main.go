package main

import (
	"context"
	"os"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/cmd"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/telemetry"
)

func main() {
	ctx := context.Background()
	shutdown, _ := telemetry.Init(ctx, "forage-ctl")
	defer shutdown()

	// Extract parent trace context from environment (e.g., TRACEPARENT
	// set by E2E test framework for cross-process trace continuity).
	ctx = telemetry.ContextFromEnv(ctx)

	if err := cmd.Execute(ctx); err != nil {
		os.Exit(errors.GetExitCode(err))
	}
}
