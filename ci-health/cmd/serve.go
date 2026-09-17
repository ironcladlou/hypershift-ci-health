package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	webassets "github.com/ironcladlou/hypershift-ci-health/ci-health/assets"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/healthreport"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
	"github.com/spf13/cobra"
)

func newServeCommand(indexHTML string) *cobra.Command {
	var (
		addr             string
		dev              bool
		jobRegistryPath  string
		reportPlanPath   string
		healthReportPath string
		observationPath  string
	)

	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve the CI health dashboard",
		RunE: func(command *cobra.Command, args []string) error {
			registry, err := jobregistry.LoadFile(jobRegistryPath)
			if err != nil {
				return err
			}
			plan, err := reportplan.LoadFile(reportPlanPath, registry)
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
			health, err := healthreport.LoadFile(healthReportPath, registry, plan, observation)
			if err != nil {
				return err
			}
			state := newApplicationState(registry, plan, health)
			server := &http.Server{Addr: addr, Handler: newHTTPHandler(indexHTML, dev, state)}
			go func() {
				<-command.Context().Done()
				server.Close()
			}()

			fmt.Fprintf(os.Stderr, "http://localhost%s\n", addr)
			fmt.Fprintf(os.Stderr, "Job registry: %s (%d jobs)\n", jobRegistryPath, len(registry.Jobs))
			fmt.Fprintf(os.Stderr, "Health report: %s (complete=%t)\n", health.GeneratedAt.Format("2006-01-02T15:04:05Z"), health.Complete)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}

	command.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	command.Flags().BoolVar(&dev, "dev", false, "Serve index.html from filesystem instead of embedded copy")
	command.Flags().StringVar(&jobRegistryPath, "job-registry", "job-registry.json", "Path to a generated job registry JSON file")
	command.Flags().StringVar(&reportPlanPath, "report-plan", "report-plan.json", "Path to a generated report plan JSON file")
	command.Flags().StringVar(&healthReportPath, "health-report", "health-report.json", "Path to an evaluated health report JSON file")
	command.Flags().StringVar(&observationPath, "sippy-observation", "sippy-observation.json", "Path to the Sippy observation used by the health report")
	return command
}

type applicationState struct {
	registry           *jobregistry.Registry
	plan               *reportplan.Plan
	registryIndex      map[string]*jobregistry.Job
	registryETag       string
	jobETags           map[string]string
	health             *healthreport.Report
	planETag           string
	healthETag         string
	apiDescriptionETag string
}

func newApplicationState(registry *jobregistry.Registry, plan *reportplan.Plan, health *healthreport.Report) *applicationState {
	registryIndex := registry.Index()
	jobETags := make(map[string]string, len(registryIndex))
	for id, job := range registryIndex {
		jobETags[id] = contentETag(job)
	}
	healthETag := ""
	if health != nil {
		healthETag = contentETag(health)
	}
	return &applicationState{
		registry:           registry,
		plan:               plan,
		registryIndex:      registryIndex,
		registryETag:       contentETag(registry),
		jobETags:           jobETags,
		health:             health,
		planETag:           contentETag(plan),
		healthETag:         healthETag,
		apiDescriptionETag: contentETag(struct{ Instance int64 }{time.Now().UnixNano()}),
	}
}

func newHTTPHandler(indexHTML string, dev bool, state *applicationState) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+fuseAssetPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", immutableAssetCacheControl)
		_, _ = w.Write(webassets.FuseJS)
	})
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
	mux.HandleFunc("/api/report-plan", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, state.plan) })
	registerJobRegistryAPI(mux, state)
	return withAPICaching(mux, state)
}

const apiCacheControl = "public, max-age=300, must-revalidate"

const (
	fuseAssetPath              = "/assets/fuse-7.5.0.min.mjs"
	immutableAssetCacheControl = "public, max-age=31536000, immutable"
)

func withAPICaching(next http.Handler, state *applicationState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.RawQuery == "" {
			if etag := state.etagForPath(r.URL.Path); etag != "" {
				w.Header().Set("Cache-Control", apiCacheControl)
				w.Header().Set("ETag", etag)
				if etagMatches(r.Header.Get("If-None-Match"), etag) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (state *applicationState) etagForPath(path string) string {
	switch path {
	case "/api/health":
		return state.healthETag
	case "/api/report-plan":
		return state.planETag
	case "/api/job-registry":
		return state.registryETag
	case "/api/openapi.json", "/api/openapi.yaml", "/api/schemas/Job.json", "/api/schemas/Registry.json":
		return state.apiDescriptionETag
	}
	const jobPath = "/api/job-registry/jobs/"
	if !strings.HasPrefix(path, jobPath) {
		return ""
	}
	id, err := url.PathUnescape(strings.TrimPrefix(path, jobPath))
	if err != nil {
		return ""
	}
	return state.jobETags[id]
}

func contentETag(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(`"%x"`, sha256.Sum256(data))
}

func etagMatches(header, etag string) bool {
	etag = strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
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

const jobRegistryAPIDescription = `The HyperShift job registry is the versioned, generated inventory of Prow jobs used by CI Health and other consumers. It provides one stable record per job and brings together:

- the job identity and source definition in ` + "`openshift/release`" + `;
- job type, repository, context, branch, version, and platform metadata;
- presubmit behavior and explicitly recorded periodic counterparts;
- release-controller participation; and
- deterministic navigation links to Prow, Sippy, and release status pages.

## Scope and authority

The registry is generated solely from a selected ` + "`openshift/release`" + ` checkout. Generation does not query live Prow, Sippy, or release-controller services. Source references identify the configuration behind each record; service URLs are navigation hints rather than evidence of current data availability.

## Using the contract

The top-level ` + "`api_version`" + ` identifies the registry schema. A job is the unit of record, identifiers are globally unique and stable, and multi-valued facts remain arrays. Unknown or inapplicable values are empty, null, or omitted rather than represented by sentinel strings.

Use the collection endpoint for discovery and local analysis, or the individual-job endpoint when a stable ID is already known. Each response links to its JSON Schema for machine discovery and validation.`

const jobRegistryTagDescription = `Browse the complete generated registry or retrieve one job by its stable Prow identifier. Both operations return the same versioned job contract and include links to discoverable JSON Schemas.`

func registerJobRegistryAPI(mux *http.ServeMux, state *applicationState) {
	config := huma.DefaultConfig("HyperShift Job Registry API", jobregistry.CurrentAPIVersion)
	config.Info.Description = jobRegistryAPIDescription
	config.Tags = []*huma.Tag{{
		Name:        "Job registry",
		Description: jobRegistryTagDescription,
		ExternalDocs: &huma.ExternalDocs{
			Description: "Registry design, discovery scope, and invariants",
			URL:         "https://github.com/ironcladlou/hypershift-ci-health/blob/main/ci-health/jobregistry/README.md",
		},
	}}
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
