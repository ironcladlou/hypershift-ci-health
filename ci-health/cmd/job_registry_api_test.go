package cmd

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/healthreport"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

func TestEmbeddedBrowserAsset(t *testing.T) {
	handler := newHTTPHandler(false, newApplicationState(&jobregistry.Registry{}, nil))
	request := httptest.NewRequest(http.MethodGet, "/assets/vendor/preact/preact.module.js", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable caching", got)
	}
	if response.Body.Len() == 0 {
		t.Error("response does not contain the embedded Preact module")
	}
}

func TestApplicationAssetsRevalidateByContent(t *testing.T) {
	handler := newHTTPHandler(false, newApplicationState(&jobregistry.Registry{}, nil))
	paths := []string{"/presubmits", "/payload", "/component-readiness", "/registry", "/assets/web/app.js", "/assets/web/favicon.svg", "/assets/web/styles.css"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			etag := response.Header().Get("ETag")
			if etag == "" {
				t.Fatal("response has no content ETag")
			}
			if strings.HasPrefix(path, "/assets/") && response.Header().Get("Cache-Control") != dataCacheControl {
				t.Errorf("Cache-Control = %q, want %q", response.Header().Get("Cache-Control"), dataCacheControl)
			}

			request = httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("If-None-Match", etag)
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNotModified {
				t.Errorf("conditional status = %d, want 304", response.Code)
			}
			if response.Body.Len() != 0 {
				t.Errorf("conditional response body has %d bytes, want 0", response.Body.Len())
			}
		})
	}
}

func TestDashboardRoutes(t *testing.T) {
	handler := newHTTPHandler(false, newApplicationState(&jobregistry.Registry{}, nil))

	request := httptest.NewRequest(http.MethodGet, "/?window=2w", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("root status = %d, want %d", response.Code, http.StatusTemporaryRedirect)
	}
	if got := response.Header().Get("Location"); got != "/presubmits?window=2w" {
		t.Errorf("Location = %q, want %q", got, "/presubmits?window=2w")
	}

	request = httptest.NewRequest(http.MethodGet, "/unknown", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("unknown route status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestResponsesUseGzip(t *testing.T) {
	handler := newHTTPHandler(false, newApplicationState(&jobregistry.Registry{}, nil))
	request := httptest.NewRequest(http.MethodGet, "/assets/vendor/preact/preact.module.js", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", response.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("decompressed response is empty")
	}
}

func TestGzipNegotiation(t *testing.T) {
	for _, test := range []struct {
		header string
		want   bool
	}{{"gzip", true}, {"br, gzip;q=0.5", true}, {"gzip;q=0", false}, {"br", false}} {
		if got := acceptsGzip(test.header); got != test.want {
			t.Errorf("acceptsGzip(%q) = %t, want %t", test.header, got, test.want)
		}
	}
}

func TestServeArtifactDefaults(t *testing.T) {
	command := newServeCommand()
	tests := map[string]string{
		"job-registry":      "job-registry.json",
		"report-plan":       "report-plan.json",
		"sippy-observation": "sippy-observation.json",
		"health-report":     "health-report.json",
	}
	for name, want := range tests {
		flag := command.Flags().Lookup(name)
		if flag == nil || flag.DefValue != want {
			t.Errorf("--%s default = %v, want %q", name, flag, want)
		}
	}
}

func TestGoldenRegistrySingleJobAPI(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	handler := newHTTPHandler(false, newApplicationState(registry, nil))
	const id = "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws"
	tests := []struct {
		name        string
		path        string
		status      int
		contentType string
		contains    string
	}{
		{"JSON", "/api/job-registry/jobs/" + id, http.StatusOK, "application/json", `"id":"` + id + `"`},
		{"missing", "/api/job-registry/jobs/missing", http.StatusNotFound, "application/problem+json", `"detail":"job not found"`},
		{"undocumented query", "/api/job-registry/jobs/" + id + "?format=yaml", http.StatusUnprocessableEntity, "application/problem+json", `"message":"unknown query parameter"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Errorf("status = %d, want %d", response.Code, test.status)
			}
			if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, test.contentType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, test.contentType)
			}
			if !strings.Contains(response.Body.String(), test.contains) {
				t.Errorf("body does not contain %q: %s", test.contains, response.Body.String())
			}
			if test.name == "JSON" {
				var job jobregistry.Job
				if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil || job.ID != id {
					t.Errorf("decode JSON job = %q, %v", job.ID, err)
				}
				if got := response.Header().Get("Link"); !strings.Contains(got, "/api/schemas/Job.json") || !strings.Contains(got, `rel="describedBy"`) {
					t.Errorf("Link = %q, want Job schema describedby link", got)
				}
				if !strings.Contains(response.Body.String(), `"$schema":`) {
					t.Errorf("body has no discoverable schema: %s", response.Body.String())
				}
			}
		})
	}
}

func TestJobRegistryDocumentationAPI(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	handler := newHTTPHandler(false, newApplicationState(registry, nil))
	tests := []struct {
		name        string
		path        string
		contentType string
		contains    []string
	}{
		{"registry", "/api/job-registry", "application/json", []string{`"api_version":"job-registry/v7"`, `"$schema":`}},
		{"Scalar docs", "/api/docs", "text/html", []string{"@scalar/api-reference", "/api/openapi.json", `data-configuration="{&#34;agent&#34;:{&#34;disabled&#34;:true},&#34;showDeveloperTools&#34;:&#34;never&#34;}"`}},
		{"OpenAPI JSON", "/api/openapi.json", "application/openapi+json", []string{`"openapi":"3.1.0"`, `"description":"The HyperShift job registry is the versioned, generated inventory`, `"description":"Browse the complete generated registry`, `"/api/job-registry"`, `"/api/job-registry/jobs/{id}"`}},
		{"OpenAPI YAML", "/api/openapi.yaml", "application/openapi+yaml", []string{"openapi: 3.1.0", "/api/job-registry:"}},
		{"Registry schema", "/api/schemas/Registry.json", "application/json", []string{`"api_version"`, `"jobs"`}},
		{"Job schema", "/api/schemas/Job.json", "application/json", []string{`"description":"Globally unique, stable identity of the Prow job"`, `"enum":["none","v1","v2"]`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, test.contentType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, test.contentType)
			}
			for _, want := range test.contains {
				if !strings.Contains(response.Body.String(), want) {
					t.Errorf("body does not contain %q: %s", want, response.Body.String())
				}
			}
		})
	}
}

func TestAPIResponseCaching(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	health := &healthreport.Report{Windows: map[string]*healthreport.WindowData{"1w": {}}}
	handler := newHTTPHandler(false, newApplicationState(registry, health))
	const id = "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws"
	paths := []string{
		"/_dashboard/health/windows/1w",
		"/api/job-registry",
		"/api/job-registry/jobs/" + id,
		"/api/openapi.json",
		"/api/schemas/Job.json",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("initial status = %d, want 200", response.Code)
			}
			if got := response.Header().Get("Cache-Control"); got != dataCacheControl {
				t.Errorf("Cache-Control = %q, want %q", got, dataCacheControl)
			}
			etag := response.Header().Get("ETag")
			if etag == "" {
				t.Fatal("initial response has no ETag")
			}

			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("If-None-Match", etag)
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNotModified {
				t.Errorf("conditional status = %d, want 304", response.Code)
			}
			if response.Body.Len() != 0 {
				t.Errorf("conditional response body has %d bytes, want 0", response.Body.Len())
			}
		})
	}
}

func TestDashboardHealthWindowAPI(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	health := &healthreport.Report{APIVersion: healthreport.CurrentAPIVersion, Windows: map[string]*healthreport.WindowData{"1w": {SparklineSlots: []string{"2026-09-17 12:00"}}}}
	handler := newHTTPHandler(false, newApplicationState(registry, health))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_dashboard/health/windows/1w", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("dashboard response exposes CORS header %q", got)
	}
	var payload healthWindowResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Window != "1w" || payload.Data == nil || len(payload.Data.SparklineSlots) != 1 {
		t.Fatalf("window response = %+v", payload)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_dashboard/health/windows/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing window status = %d", response.Code)
	}
}

func TestHealthProbes(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	tests := []struct {
		name   string
		path   string
		health *healthreport.Report
		want   int
	}{
		{"live before data", "/livez", nil, http.StatusOK},
		{"not ready before data", "/readyz", nil, http.StatusServiceUnavailable},
		{"ready with data", "/readyz", &healthreport.Report{}, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHTTPHandler(false, newApplicationState(registry, test.health))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.want {
				t.Errorf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}
