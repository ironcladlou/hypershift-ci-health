package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

func ExecuteContext(ctx context.Context, indexHTML string) error {
	return NewRootCommand(indexHTML).ExecuteContext(ctx)
}

func NewRootCommand(indexHTML string) *cobra.Command {
	root := &cobra.Command{
		Use:   "ci-health",
		Short: "HyperShift CI health dashboard",
	}
	root.AddCommand(newServeCommand(indexHTML))
	root.AddCommand(newJobRegistryCommand())
	return root
}
