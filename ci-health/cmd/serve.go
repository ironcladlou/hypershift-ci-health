package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

func newServeCommand(indexHTML string) *cobra.Command {
	var (
		addr            string
		dev             bool
		jobRegistryPath string
	)

	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve the CI health dashboard",
		RunE: func(command *cobra.Command, args []string) error {
			registry, err := jobregistry.LoadFile(jobRegistryPath)
			if err != nil {
				return err
			}
			catalog, err := jobs.NewCatalog(registry)
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
	mux.HandleFunc("/api/job-registry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, state.registry)
	})
	mux.HandleFunc("GET /api/job-registry/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job := state.registryIndex[r.PathValue("id")]
		if job == nil {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("format") {
		case "", "json":
			writeJSON(w, job)
		case "yaml":
			writeYAML(w, job)
		default:
			http.Error(w, "unsupported registry entry format", http.StatusBadRequest)
		}
	})
	return mux
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(value)
}

func writeYAML(w http.ResponseWriter, value any) {
	jsonData, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "encode registry entry", http.StatusInternalServerError)
		return
	}
	var document yaml.Node
	if err := yaml.Unmarshal(jsonData, &document); err != nil {
		http.Error(w, "encode registry entry", http.StatusInternalServerError)
		return
	}
	clearYAMLStyle(&document)
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer encoder.Close()
	if len(document.Content) > 0 {
		_ = encoder.Encode(document.Content[0])
	}
}

func clearYAMLStyle(node *yaml.Node) {
	node.Style = 0
	for _, child := range node.Content {
		clearYAMLStyle(child)
	}
}
