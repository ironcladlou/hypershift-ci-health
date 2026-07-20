package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/retests"
	"github.com/spf13/cobra"
)

//go:embed index.html
var indexHTML string

func main() {
	rootCmd := &cobra.Command{
		Use:   "ci-health",
		Short: "HyperShift CI health dashboard and analysis tools",
	}

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(retestsCmd())

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

func serveCmd() *cobra.Command {
	var (
		addr            string
		dev             bool
		org             string
		repo            string
		githubToken     string
		githubTokenFile string
		window          int
		concurrency     int
		interval        time.Duration
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the CI health dashboard with live retest analysis",
		Long: `Serves the static dashboard files and runs retest analysis in the
background on a configurable interval. Results are available at /api/retests.

A GitHub token is required for the retest analyzer.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken(githubToken, githubTokenFile)
			if err != nil {
				return err
			}

			cfg := retests.Config{
				Org:         org,
				Repo:        repo,
				GitHubToken: token,
				WindowDays:  window,
				Concurrency: concurrency,
			}

			provider := retests.NewProvider(cmd.Context(), cfg, interval)

			mux := http.NewServeMux()
			if dev {
				fmt.Fprintf(os.Stderr, "Dev mode: serving index.html from filesystem\n")
				mux.Handle("/", http.FileServer(http.Dir(".")))
			} else {
				mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/" {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write([]byte(indexHTML))
				})
			}
			mux.HandleFunc("/api/retests", func(w http.ResponseWriter, r *http.Request) {
				data := provider.Data()
				if data == nil {
					http.Error(w, "analysis not yet available", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				json.NewEncoder(w).Encode(data)
			})

			server := &http.Server{Addr: addr, Handler: mux}
			go func() {
				<-cmd.Context().Done()
				server.Close()
			}()

			fmt.Fprintf(os.Stderr, "http://localhost%s\n", addr)
			fmt.Fprintf(os.Stderr, "Retest analysis every %s (window: %dd)\n", interval, window)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	cmd.Flags().BoolVar(&dev, "dev", false, "Serve index.html from filesystem instead of embedded copy")
	cmd.Flags().StringVar(&org, "org", "openshift", "GitHub organization")
	cmd.Flags().StringVar(&repo, "repo", "hypershift", "GitHub repository")
	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub API token (or set GITHUB_TOKEN)")
	cmd.Flags().StringVar(&githubTokenFile, "github-token-file", "", "Path to file containing GitHub API token")
	cmd.Flags().IntVar(&window, "window", 7, "Lookback window in days")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Parallel Prow fetches")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Minute, "Retest analysis refresh interval")

	return cmd
}

func retestsCmd() *cobra.Command {
	var (
		org             string
		repo            string
		githubToken     string
		githubTokenFile string
		window          int
		output          string
		concurrency     int
	)

	cmd := &cobra.Command{
		Use:   "retests",
		Short: "Analyze retest frequency for recently merged PRs",
		Long: `Scrapes Prow pr-history pages for recently merged PRs to determine
how many retests each PR needed before all blocking presubmits passed.

Produces a JSON analysis with per-PR retest counts and aggregate statistics
including first-try merge probability.

Set --github-token, --github-token-file, or GITHUB_TOKEN to authenticate.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken(githubToken, githubTokenFile)
			if err != nil {
				return err
			}

			cfg := retests.Config{
				Org:         org,
				Repo:        repo,
				GitHubToken: token,
				WindowDays:  window,
				Concurrency: concurrency,
			}

			result, err := retests.Run(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			var w *os.File
			if output == "" || output == "-" {
				w = os.Stdout
			} else {
				w, err = os.Create(output)
				if err != nil {
					return fmt.Errorf("creating output file: %w", err)
				}
				defer w.Close()
			}

			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			if err := enc.Encode(result); err != nil {
				return fmt.Errorf("writing JSON: %w", err)
			}

			if output != "" && output != "-" {
				fmt.Fprintf(os.Stderr, "Wrote %s\n", output)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "openshift", "GitHub organization")
	cmd.Flags().StringVar(&repo, "repo", "hypershift", "GitHub repository")
	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub API token (or set GITHUB_TOKEN)")
	cmd.Flags().StringVar(&githubTokenFile, "github-token-file", "", "Path to file containing GitHub API token")
	cmd.Flags().IntVar(&window, "window", 7, "Lookback window in days")
	cmd.Flags().StringVar(&output, "output", "", "Output file (default: stdout)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Parallel Prow fetches")

	return cmd
}

func resolveToken(flag, file string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading token file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("GitHub token required: set --github-token, --github-token-file, or GITHUB_TOKEN")
}
