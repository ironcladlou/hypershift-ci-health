package cmd

import (
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/spf13/cobra"
)

func newJobRegistryCommand() *cobra.Command {
	var (
		releaseDir           string
		sippyBaseURL         string
		sippyStreamBaseURL   string
		prowBaseURL          string
		releaseStatusBaseURL string
		pretty               bool
	)

	command := &cobra.Command{
		Use:   "job-registry",
		Short: "Generate the HyperShift job registry",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			registry, err := jobregistry.Discover(releaseDir, jobregistry.Options{
				SippyBaseURL:         sippyBaseURL,
				SippyStreamBaseURL:   sippyStreamBaseURL,
				ProwBaseURL:          prowBaseURL,
				ReleaseStatusBaseURL: releaseStatusBaseURL,
			})
			if err != nil {
				return err
			}

			return writeArtifactJSON(command, registry, "job registry", pretty)
		},
	}

	command.Flags().StringVar(&releaseDir, "release-dir", "", "Path to an openshift/release checkout")
	command.MarkFlagRequired("release-dir")
	command.Flags().StringVar(&sippyBaseURL, "sippy-base-url", "", "Base URL for generated Sippy job links")
	command.Flags().StringVar(&sippyStreamBaseURL, "sippy-stream-base-url", "", "Base URL for generated Sippy release stream links")
	command.Flags().StringVar(&prowBaseURL, "prow-base-url", "", "Base URL for generated Prow job history links")
	command.Flags().StringVar(&releaseStatusBaseURL, "release-status-base-url", "", "Base URL for generated release payload status links")
	command.Flags().BoolVar(&pretty, "pretty", false, "Indent JSON for human review")
	return command
}
