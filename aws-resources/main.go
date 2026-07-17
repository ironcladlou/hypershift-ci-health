package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
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
  render     Read a JSON data file and output table, json, or html
  serve      Serve the dashboard (static or with live collection)
  setup      Provision IAM role and cluster-specific Kustomize patches
  teardown   Remove IAM role and generated patch files

Examples:
  aws-resources collect
  aws-resources collect --regions us-east-1 --output snapshot.json
  aws-resources render --format html > index.html
  aws-resources serve                          # static file server
  aws-resources serve --collect                # live collection mode
  aws-resources serve --collect --interval 15m
  aws-resources setup --dry-run
  aws-resources teardown`,
	}

	rootCmd.AddCommand(collectCmd())
	rootCmd.AddCommand(renderCmd())
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
	cfg := DefaultConfig()
	var regionsFlag, outputFile string

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Discover tagged AWS resources and check Prow job status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regionsFlag != "" {
				cfg.Regions = strings.Split(regionsFlag, ",")
			}
			return runCollect(cmd.Context(), cfg, outputFile)
		},
	}

	cmd.Flags().StringVar(&regionsFlag, "regions", "", "Comma-separated AWS regions (default: all US regions)")
	cmd.Flags().StringVar(&cfg.JobID, "job-id", "", "Filter to a specific prow job ID")
	cmd.Flags().StringVar(&outputFile, "output", "data.json", "Output file path for the JSON data")

	return cmd
}

func renderCmd() *cobra.Command {
	var inputFile, format string

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render a report from a previously collected JSON data file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRender(inputFile, format)
		},
	}

	cmd.Flags().StringVar(&inputFile, "input", "data.json", "Input JSON data file")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json, or html")

	return cmd
}

func serveCmd() *cobra.Command {
	cfg := DefaultConfig()
	var regionsFlag, addr string
	var interval time.Duration
	var collect bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the dashboard, optionally with periodic collection",
		Long: `Serves the HTML dashboard and data API. With --collect, periodically
discovers AWS resources and refreshes the in-memory data store.

Without --collect, serves static files from the current directory
(for local development).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if regionsFlag != "" {
				cfg.Regions = strings.Split(regionsFlag, ",")
			}
			if collect {
				return runLiveServe(cmd.Context(), cfg, addr, interval)
			}
			return runStaticServe(cmd.Context(), ".", addr)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	cmd.Flags().BoolVar(&collect, "collect", false, "Enable periodic AWS resource collection")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Minute, "Collection interval (with --collect)")
	cmd.Flags().StringVar(&regionsFlag, "regions", "", "Comma-separated AWS regions (default: all US regions)")
	cmd.Flags().StringVar(&cfg.JobID, "job-id", "", "Filter to a specific prow job ID")

	return cmd
}

func collectData(ctx context.Context, cfg Config) (*APIResponse, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	graph := &ResourceGraph{}
	for _, region := range cfg.Regions {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("loading AWS config for %s: %w", region, err)
		}

		taggingClient := resourcegroupstaggingapi.NewFromConfig(awsCfg)

		fmt.Fprintf(os.Stderr, "Discovering tagged resources in %s...\n", region)
		regionGraph, err := Discover(ctx, taggingClient, cfg.JobID)
		if err != nil {
			return nil, fmt.Errorf("discovery failed in %s: %w", region, err)
		}

		graph.Merge(regionGraph)
	}

	if len(graph.Jobs) > 0 {
		fmt.Fprintf(os.Stderr, "Checking %d prow job(s) for terminal state...\n", len(graph.Jobs))
		CheckProwJobs(ctx, httpClient, graph)
		graph.Sort()
	}

	return &APIResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     graph.Summary(),
		Jobs:        graph.Jobs,
	}, nil
}

func runCollect(ctx context.Context, cfg Config, outputFile string) error {
	resp, err := collectData(ctx, cfg)
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
}

func runRender(inputFile, format string) error {
	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("opening input file: %w", err)
	}
	defer f.Close()

	var resp APIResponse
	if err := json.NewDecoder(f).Decode(&resp); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}

	graph := &ResourceGraph{Jobs: resp.Jobs}

	switch format {
	case "json":
		return PrintJSON(os.Stdout, graph)
	case "html":
		return PrintHTML(os.Stdout)
	default:
		PrintTable(os.Stdout, graph)
		PrintSummary(os.Stdout, graph)
	}

	return nil
}

func runLiveServe(ctx context.Context, cfg Config, addr string, interval time.Duration) error {
	var (
		mu       sync.RWMutex
		dataJSON []byte
	)

	refresh := func() {
		fmt.Fprintf(os.Stderr, "Collecting data...\n")
		resp, err := collectData(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Collection error: %v\n", err)
			return
		}
		b, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Marshal error: %v\n", err)
			return
		}
		mu.Lock()
		dataJSON = b
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "Collection complete: %d jobs, %d resources\n", resp.Summary.TotalJobs, resp.Summary.TotalResources)
	}

	refresh()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		PrintHTML(w)
	})
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		d := dataJSON
		mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(interval.Seconds())))
		w.Write(d)
	})

	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		server.Close()
	}()

	fmt.Fprintf(os.Stderr, "Serving dashboard on %s (collecting every %s)\n", addr, interval)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runStaticServe(ctx context.Context, dir, addr string) error {
	fs := http.FileServer(http.Dir(dir))
	server := &http.Server{Addr: addr, Handler: fs}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	url := "http://localhost" + addr
	fmt.Fprintf(os.Stderr, "Serving %s at %s\n", dir, url)
	openBrowser(url)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
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
