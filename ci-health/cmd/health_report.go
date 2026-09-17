package cmd

import (
	"fmt"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/healthreport"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
	"github.com/spf13/cobra"
)

func newHealthReportCommand() *cobra.Command {
	var registryPath, planPath, observationPath string
	command := &cobra.Command{Use: "health-report", Short: "Evaluate a cached health report from registry, plan, and Sippy observation", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		registry, err := jobregistry.LoadFile(registryPath)
		if err != nil {
			return err
		}
		plan, err := reportplan.LoadFile(planPath, registry)
		if err != nil {
			return err
		}
		planDigest, err := reportplan.Digest(plan)
		if err != nil {
			return err
		}
		observation, err := sippy.LoadObservationFile(observationPath, planDigest)
		if err != nil {
			return err
		}
		report, err := healthreport.Evaluate(registry, plan, observation)
		if err != nil {
			return fmt.Errorf("evaluate health report: %w", err)
		}
		return writeArtifactJSON(command, report, "health report", false)
	}}
	command.Flags().StringVar(&registryPath, "job-registry", "", "Path to a generated job registry")
	command.Flags().StringVar(&planPath, "report-plan", "", "Path to a generated report plan")
	command.Flags().StringVar(&observationPath, "sippy-observation", "", "Path to a Sippy observation")
	command.MarkFlagRequired("job-registry")
	command.MarkFlagRequired("report-plan")
	command.MarkFlagRequired("sippy-observation")
	return command
}
