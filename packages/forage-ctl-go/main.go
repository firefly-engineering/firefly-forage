package main

import (
	"os"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/cmd"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/errors"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(errors.GetExitCode(err))
	}
}
