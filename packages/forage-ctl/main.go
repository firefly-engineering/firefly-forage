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

	if err := cmd.Execute(); err != nil {
		os.Exit(errors.GetExitCode(err))
	}
}
