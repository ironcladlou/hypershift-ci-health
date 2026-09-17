package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
	"github.com/spf13/cobra"
)

func newServeCommand(indexHTML string) *cobra.Command {
	var (
		addr               string
		dev                bool
		jobRegistryPath    string
		developmentBranch  string
		developmentRelease string
	)

	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve the CI health dashboard",
		RunE: func(command *cobra.Command, args []string) error {
			registry, err := jobregistry.LoadFile(jobRegistryPath)
			if err != nil {
				return err
			}
			catalog, err := jobs.NewCatalog(registry, jobs.CatalogOptions{
				DevelopmentBranch:  developmentBranch,
				DevelopmentRelease: developmentRelease,
			})
			if err != nil {
				return fmt.Errorf("validate dashboard job configuration: %w", err)
			}

			health, status, err := sippy.CollectSnapshot(command.Context(), sippy.NewClient(), catalog)
			if err != nil {
				return fmt.Errorf("collect Sippy health snapshot: %w", err)
			}
			state := newApplicationState(registry, health, status)
			server := &http.Server{Addr: addr, Handler: newHTTPHandler(indexHTML, dev, state)}
			go func() {
				<-command.Context().Done()
				server.Close()
			}()

			fmt.Fprintf(os.Stderr, "http://localhost%s\n", addr)
			fmt.Fprintf(os.Stderr, "Job registry: %s (%d jobs)\n", jobRegistryPath, len(registry.Jobs))
			fmt.Fprintf(os.Stderr, "Sippy snapshot: %s\n", health.GeneratedAt.Format("2006-01-02T15:04:05Z"))
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}

	command.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	command.Flags().BoolVar(&dev, "dev", false, "Serve index.html from filesystem instead of embedded copy")
	command.Flags().StringVar(&jobRegistryPath, "job-registry", "", "Path to a generated job registry JSON file")
	command.Flags().StringVar(&developmentBranch, "development-branch", "main", "Development branch represented by the dashboard")
	command.Flags().StringVar(&developmentRelease, "development-release", "5.1", "Development release represented by the dashboard")
	command.MarkFlagRequired("job-registry")
	return command
}

type applicationState struct {
	registry      *jobregistry.Registry
	registryIndex map[string]*jobregistry.Job
	health        *sippy.HealthSnapshot
	status        sippy.CollectionStatus
}

func newApplicationState(registry *jobregistry.Registry, health *sippy.HealthSnapshot, status sippy.CollectionStatus) *applicationState {
	return &applicationState{
		registry:      registry,
		registryIndex: registry.Index(),
		health:        health,
		status:        status,
	}
}

func newHTTPHandler(indexHTML string, dev bool, state *applicationState) http.Handler {
	mux := http.NewServeMux()
	if dev {
		fmt.Fprintln(os.Stderr, "Dev mode: serving index.html from filesystem")
		mux.Handle("/", http.FileServer(http.Dir(".")))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(indexHTML))
		})
	}
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if state.health == nil {
			http.Error(w, "health snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if state.health == nil {
			http.Error(w, "data not yet available", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, state.health)
	})
	mux.HandleFunc("/api/health/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, state.status)
	})
	registerJobRegistryAPI(mux, state)
	return mux
}

type getJobRegistryOutput struct {
	Body jobregistry.Registry
}

type getJobInput struct {
	ID string `path:"id" doc:"Globally unique, stable Prow job identifier"`
}

type getJobOutput struct {
	Body jobregistry.Job
}

func registerJobRegistryAPI(mux *http.ServeMux, state *applicationState) {
	config := huma.DefaultConfig("HyperShift Job Registry API", jobregistry.CurrentAPIVersion)
	config.OpenAPIPath = "/api/openapi"
	config.SchemasPath = "/api/schemas"
	config.DocsPath = "/api/docs"
	config.DocsRenderer = huma.DocsRendererScalar
	config.DocsRendererConfig = map[string]any{
		"agent": map[string]any{
			"disabled": true,
		},
		"showDeveloperTools": "never",
	}
	config.RejectUnknownQueryParameters = true
	api := humago.New(mux, config)

	huma.Register(api, huma.Operation{
		OperationID: "get-job-registry",
		Method:      http.MethodGet,
		Path:        "/api/job-registry",
		Summary:     "Get the complete job registry",
		Description: "Returns the generated registry contract and every discovered HyperShift Prow job.",
		Tags:        []string{"Job registry"},
	}, func(_ context.Context, _ *struct{}) (*getJobRegistryOutput, error) {
		return &getJobRegistryOutput{Body: *state.registry}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job",
		Method:      http.MethodGet,
		Path:        "/api/job-registry/jobs/{id}",
		Summary:     "Get one registry job",
		Description: "Returns the registry entry identified by its stable Prow job ID.",
		Tags:        []string{"Job registry"},
		Errors:      []int{http.StatusNotFound},
	}, func(_ context.Context, input *getJobInput) (*getJobOutput, error) {
		job := state.registryIndex[input.ID]
		if job == nil {
			return nil, huma.Error404NotFound("job not found")
		}
		return &getJobOutput{Body: *job}, nil
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(value)
}
