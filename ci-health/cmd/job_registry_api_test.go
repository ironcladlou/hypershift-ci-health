package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
)

func TestGoldenRegistrySingleJobAPI(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	handler := newHTTPHandler("", false, newApplicationState(registry, nil, sippy.CollectionStatus{}))
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
	handler := newHTTPHandler("", false, newApplicationState(registry, nil, sippy.CollectionStatus{}))
	tests := []struct {
		name        string
		path        string
		contentType string
		contains    []string
	}{
		{"registry", "/api/job-registry", "application/json", []string{`"api_version":"job-registry/v7"`, `"$schema":`}},
		{"Scalar docs", "/api/docs", "text/html", []string{"@scalar/api-reference", "/api/openapi.json", `data-configuration="{&#34;agent&#34;:{&#34;disabled&#34;:true},&#34;showDeveloperTools&#34;:&#34;never&#34;}"`}},
		{"OpenAPI JSON", "/api/openapi.json", "application/openapi+json", []string{`"openapi":"3.1.0"`, `"/api/job-registry"`, `"/api/job-registry/jobs/{id}"`}},
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

func TestHealthProbes(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	tests := []struct {
		name   string
		path   string
		health *sippy.HealthSnapshot
		want   int
	}{
		{"live before data", "/livez", nil, http.StatusOK},
		{"not ready before data", "/readyz", nil, http.StatusServiceUnavailable},
		{"ready with data", "/readyz", &sippy.HealthSnapshot{}, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHTTPHandler("", false, newApplicationState(registry, test.health, sippy.CollectionStatus{}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.want {
				t.Errorf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}
