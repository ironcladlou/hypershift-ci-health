package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

func ExecuteContext(ctx context.Context) error {
	return NewRootCommand().ExecuteContext(ctx)
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "ci-health",
		Short: "HyperShift CI health dashboard",
	}
	root.AddCommand(newServeCommand())
	root.AddCommand(newJobRegistryCommand())
	root.AddCommand(newReportPlanCommand())
	root.AddCommand(newSippyObservationCommand())
	root.AddCommand(newHealthReportCommand())
	return root
}
