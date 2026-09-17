package cmd

import (
	"fmt"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
	"github.com/spf13/cobra"
)

func newSippyObservationCommand() *cobra.Command {
	var registryPath, planPath string
	command := &cobra.Command{Use: "sippy-observation", Short: "Collect an immutable Sippy observation for a report plan", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		registry, err := jobregistry.LoadFile(registryPath)
		if err != nil {
			return err
		}
		plan, err := reportplan.LoadFile(planPath, registry)
		if err != nil {
			return err
		}
		observation, status, err := sippy.CollectObservation(command.Context(), sippy.NewClient(), registry, plan)
		if err != nil {
			return fmt.Errorf("collect Sippy observation: %w", err)
		}
		fmt.Fprintf(command.ErrOrStderr(), "Sippy observation: %s (%d/%d requests failed)\n", status.State, status.Failed, status.Total)
		return writeIndentedJSON(command, observation, "Sippy observation")
	}}
	command.Flags().StringVar(&registryPath, "job-registry", "", "Path to a generated job registry")
	command.Flags().StringVar(&planPath, "report-plan", "", "Path to a generated report plan")
	command.MarkFlagRequired("job-registry")
	command.MarkFlagRequired("report-plan")
	return command
}
