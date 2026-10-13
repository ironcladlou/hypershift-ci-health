package cmd

import (
	"fmt"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobpolicy"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/releaseconfig"
	"github.com/spf13/cobra"
)

func newJobPolicyCommand() *cobra.Command {
	var registryPath, policyPath, developmentBranch, developmentRelease string
	var pretty bool
	command := &cobra.Command{Use: "job-policy", Short: "Evaluate team policy over all registry jobs without collecting health data", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		registry, err := jobregistry.LoadFile(registryPath)
		if err != nil {
			return err
		}
		policy, err := jobpolicy.DefaultPolicy()
		if policyPath != "" {
			policy, err = jobpolicy.LoadPolicyFile(policyPath)
		}
		if err != nil {
			return fmt.Errorf("load team policy: %w", err)
		}
		if command.Flags().Changed("development-branch") || policyPath != "" {
			if policyPath != "" && policy.DevelopmentBranch != developmentBranch {
				return fmt.Errorf("team policy development branch %q conflicts with pipeline branch %q", policy.DevelopmentBranch, developmentBranch)
			}
			policy.DevelopmentBranch = developmentBranch
		}
		if command.Flags().Changed("development-release") || policyPath != "" {
			if policyPath != "" && policy.DevelopmentRelease != developmentRelease {
				return fmt.Errorf("team policy development release %q conflicts with pipeline release %q", policy.DevelopmentRelease, developmentRelease)
			}
			policy.DevelopmentRelease = developmentRelease
		}
		catalog, err := jobpolicy.Evaluate(registry, policy)
		if err != nil {
			return fmt.Errorf("evaluate team policy: %w", err)
		}
		return writeArtifactJSON(command, catalog, "team job catalog", pretty)
	}}
	command.Flags().StringVar(&registryPath, "job-registry", "", "Path to a generated job registry")
	command.Flags().StringVar(&policyPath, "policy", "", "Reviewed team policy YAML; defaults to the embedded HyperShift policy")
	command.Flags().BoolVar(&pretty, "pretty", false, "Indent JSON for human review")
	command.Flags().StringVar(&developmentBranch, "development-branch", releaseconfig.DevelopmentBranch, "Development branch; must agree with a supplied policy")
	command.Flags().StringVar(&developmentRelease, "development-release", releaseconfig.DevelopmentRelease, "Development release; must agree with a supplied policy")
	command.MarkFlagRequired("job-registry")
	return command
}
