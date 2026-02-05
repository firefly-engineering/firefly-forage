package main

import (
	"os"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
