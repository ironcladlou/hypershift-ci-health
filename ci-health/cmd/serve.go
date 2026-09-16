package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

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
		sippyInterval   time.Duration
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

			sippyProvider := sippy.NewProvider(command.Context(), sippy.NewClient(), catalog, sippyInterval)
			states := newApplicationStateStore(newApplicationState(registry, catalog, sippyProvider))
			server := &http.Server{Addr: addr, Handler: newHTTPHandler(indexHTML, dev, states)}
			go func() {
				<-command.Context().Done()
				server.Close()
			}()

			fmt.Fprintf(os.Stderr, "http://localhost%s\n", addr)
			fmt.Fprintf(os.Stderr, "Job registry: %s (%d jobs)\n", jobRegistryPath, len(registry.Jobs))
			fmt.Fprintf(os.Stderr, "Sippy data refresh every %s\n", sippyInterval)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}

	command.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	command.Flags().BoolVar(&dev, "dev", false, "Serve index.html from filesystem instead of embedded copy")
	command.Flags().StringVar(&jobRegistryPath, "job-registry", "", "Path to a generated job registry JSON file")
	command.Flags().DurationVar(&sippyInterval, "sippy-interval", 15*time.Minute, "Sippy data refresh interval")
	command.MarkFlagRequired("job-registry")
	return command
}

type applicationState struct {
	registry      *jobregistry.Registry
	catalog       *jobs.Catalog
	registryIndex map[string]*jobregistry.Job
	provider      *sippy.Provider
}

type applicationStateStore struct {
	current atomic.Pointer[applicationState]
}

func newApplicationState(registry *jobregistry.Registry, catalog *jobs.Catalog, provider *sippy.Provider) *applicationState {
	return &applicationState{
		registry:      registry,
		catalog:       catalog,
		registryIndex: registry.Index(),
		provider:      provider,
	}
}

func newApplicationStateStore(state *applicationState) *applicationStateStore {
	store := &applicationStateStore{}
	store.replace(state)
	return store
}

func (s *applicationStateStore) load() *applicationState {
	return s.current.Load()
}

func (s *applicationStateStore) replace(state *applicationState) {
	s.current.Store(state)
}

func newHTTPHandler(indexHTML string, dev bool, states *applicationStateStore) http.Handler {
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
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		data := states.load().provider.Data()
		if data == nil {
			http.Error(w, "data not yet available", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, data)
	})
	mux.HandleFunc("/api/health/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, states.load().provider.Status())
	})
	mux.HandleFunc("/api/job-registry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, states.load().registry)
	})
	mux.HandleFunc("GET /api/job-registry/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job := states.load().registryIndex[r.PathValue("id")]
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
