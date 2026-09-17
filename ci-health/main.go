package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
