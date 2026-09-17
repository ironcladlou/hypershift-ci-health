package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/spf13/cobra"
)

func newReportPlanCommand() *cobra.Command {
	var registryPath, developmentBranch, developmentRelease string
	command := &cobra.Command{Use: "report-plan", Short: "Build a deterministic report plan from a job registry", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		registry, err := jobregistry.LoadFile(registryPath)
		if err != nil {
			return err
		}
		plan, err := reportplan.Build(registry, reportplan.DefaultSelectionPolicy(developmentBranch, developmentRelease))
		if err != nil {
			return fmt.Errorf("build report plan: %w", err)
		}
		return writeIndentedJSON(command, plan, "report plan")
	}}
	command.Flags().StringVar(&registryPath, "job-registry", "", "Path to a generated job registry")
	command.Flags().StringVar(&developmentBranch, "development-branch", "main", "Development branch represented by the report")
	command.Flags().StringVar(&developmentRelease, "development-release", "5.1", "Development release represented by the report")
	command.MarkFlagRequired("job-registry")
	return command
}

func writeIndentedJSON(command *cobra.Command, value any, description string) error {
	encoder := json.NewEncoder(command.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("render %s JSON: %w", description, err)
	}
	return nil
}
