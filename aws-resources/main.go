package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/dmace/hypershift-ci-health/aws-resources/collector"
	"github.com/dmace/hypershift-ci-health/aws-resources/server"
	"github.com/dmace/hypershift-ci-health/aws-resources/setup"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aws-resources",
		Short: "Reconcile AWS resources tagged by e2e tests with their Prow job status",
		Long: `Finds all AWS resources tagged with hypershift.openshift.io/prow-job-id,
checks the corresponding Prow job's completion status via the Prow API,
and identifies orphaned resources (those whose Prow job has terminated).

Subcommands:
  collect    Discover resources and check Prow status, write JSON data file
  serve      Serve the dashboard with periodic collection or from a data file
  setup      Provision IAM role and cluster-specific Kustomize patches
  teardown   Remove IAM role and generated patch files

Examples:
  aws-resources collect
  aws-resources collect --regions us-east-1 --output snapshot.json
  aws-resources serve
  aws-resources serve --interval 15m
  aws-resources serve --data-file data.json
  aws-resources setup --dry-run
  aws-resources teardown`,
	}

	rootCmd.AddCommand(collectCmd())
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(setupCmd())
	rootCmd.AddCommand(teardownCmd())

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func collectCmd() *cobra.Command {
	cfg := collector.DefaultConfig()
	var regionsFlag, outputFile string

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Discover tagged AWS resources and check Prow job status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regionsFlag != "" {
				cfg.Regions = strings.Split(regionsFlag, ",")
			}

			resp, err := collector.Collect(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			if len(resp.Jobs) == 0 {
				fmt.Fprintln(os.Stderr, "No resources found with prow-job-id tags.")
			}

			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			defer f.Close()

			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(resp); err != nil {
				return fmt.Errorf("writing JSON: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Wrote %s\n", outputFile)
			return nil
		},
	}

	cmd.Flags().StringVar(&regionsFlag, "regions", "", "Comma-separated AWS regions (default: all US regions)")
	cmd.Flags().StringVar(&cfg.JobID, "job-id", "", "Filter to a specific prow job ID")
	cmd.Flags().StringVar(&outputFile, "output", "data.json", "Output file path for the JSON data")

	return cmd
}

func serveCmd() *cobra.Command {
	cfg := collector.DefaultConfig()
	var regionsFlag, addr, dataFile string
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the dashboard with periodic collection or from a data file",
		Long: `Serves the HTML dashboard and data API. By default, periodically
discovers AWS resources and refreshes the in-memory data store.

With --data-file, serves data from a pre-collected JSON file instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if regionsFlag != "" {
				cfg.Regions = strings.Split(regionsFlag, ",")
			}

			var provider server.DataProvider
			if dataFile != "" {
				provider = server.NewFileProvider(dataFile)
				fmt.Fprintf(os.Stderr, "Serving data from %s\n", dataFile)
				openBrowser("http://localhost" + addr)
			} else {
				provider = server.NewLiveProvider(cmd.Context(), cfg, interval)
				fmt.Fprintf(os.Stderr, "Collecting every %s\n", interval)
			}

			return server.Run(cmd.Context(), addr, provider)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Minute, "Collection interval")
	cmd.Flags().StringVar(&regionsFlag, "regions", "", "Comma-separated AWS regions (default: all US regions)")
	cmd.Flags().StringVar(&cfg.JobID, "job-id", "", "Filter to a specific prow job ID")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "Serve data from a JSON file instead of collecting")

	return cmd
}

func setupCmd() *cobra.Command {
	var (
		oidcProviderARN string
		namespace       string
		serviceAccount  string
		dryRun          bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Provision IAM role and cluster-specific Kustomize patches",
		Long: `Idempotently provisions everything needed for in-cluster deployment:

IAM: Creates (or updates) the IAM role and inline policy for IRSA-based
AWS access. The OIDC provider is auto-discovered from the current
kubeconfig's cluster unless --oidc-provider-arn is set. The AWS account
ID is auto-detected via sts:GetCallerIdentity. Resources use deterministic
names and are tagged for positive identification:
  Role:   hypershift-ci-aws-resources  (path /hypershift-ci/)
  Policy: orphan-resource-discovery    (inline on the role)

Route: Discovers the cluster's ingress domain and writes a Kustomize
patch with the route hostname.

Both patches are gitignored. This command is safe to re-run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.RunSetup(cmd.Context(), oidcProviderARN, namespace, serviceAccount, dryRun)
		},
	}

	cmd.Flags().StringVar(&oidcProviderARN, "oidc-provider-arn", "", "OIDC provider ARN (auto-discovered from cluster if omitted)")
	cmd.Flags().StringVar(&namespace, "namespace", "ci-health-aws-resources", "Kubernetes namespace for the trust policy")
	cmd.Flags().StringVar(&serviceAccount, "service-account", "aws-resources", "Kubernetes ServiceAccount name for the trust policy")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")

	return cmd
}

func teardownCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove IAM role and generated patch files",
		Long: `Removes the IAM role, inline policy, and generated Kustomize patch
files created by the setup command. Refuses to delete the IAM role if its
tags don't match (prevents accidental deletion of unrelated roles).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.RunTeardown(cmd.Context(), dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")

	return cmd
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Start()
}
