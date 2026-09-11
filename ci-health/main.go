package main

import (
	"context"
	_ "embed"
	"os"
	"os/signal"
	"syscall"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/cmd"
)

//go:embed index.html
var indexHTML string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := cmd.ExecuteContext(ctx, indexHTML); err != nil {
		os.Exit(1)
	}
}
