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
		{"explicit JSON", "/api/job-registry/jobs/" + id + "?format=json", http.StatusOK, "application/json", `"id":"` + id + `"`},
		{"YAML", "/api/job-registry/jobs/" + id + "?format=yaml", http.StatusOK, "application/yaml", "id: " + id},
		{"missing", "/api/job-registry/jobs/missing", http.StatusNotFound, "text/plain", "404 page not found"},
		{"unsupported format", "/api/job-registry/jobs/" + id + "?format=toml", http.StatusBadRequest, "text/plain", "unsupported registry entry format"},
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
