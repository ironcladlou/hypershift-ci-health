package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobpolicy"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

func TestTeamJobAPI(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := jobpolicy.DefaultPolicy()
	catalog, err := jobpolicy.Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	state := newApplicationState(registry, nil)
	if err := state.setJobPolicy(catalog); err != nil {
		t.Fatal(err)
	}
	handler := newHTTPHandler(false, state)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/teams/hypershift/jobs", nil))
	if response.Code != 200 {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var body teamCatalog
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) != len(registry.Jobs) || body.AlgorithmVersion != jobpolicy.AlgorithmVersion || body.PolicyDigest != catalog.PolicyDigest || body.SnapshotDigest == "" {
		t.Fatal("missing catalog provenance or coverage")
	}
	for i, entry := range body.Jobs {
		if entry.Job.ID != catalog.Jobs[i].JobID || entry.TeamMetadata.Importance.Rank != i+1 {
			t.Fatalf("wrong job or rank at %d", i)
		}
		objective := entry.TeamMetadata.HealthObjective
		if objective.InsufficientEvidence != "indeterminate" {
			t.Fatalf("missing evidence contract for %s", entry.Job.ID)
		}
		if entry.Job.Type == jobregistry.JobTypePresubmit {
			if objective.Assessment != jobpolicy.AssessmentPresubmitSystemicHealth || objective.Checks.PassRate != nil || objective.Checks.JobPermafailing == nil || objective.Checks.CorrelatedFailures == nil || objective.Attribution.Require != "unrelated-to-pr-contents" {
				t.Fatalf("wrong presubmit objective: %+v", objective)
			}
			cohort, found := body.Policy.Cohorts[objective.Checks.CorrelatedFailures.Cohort]
			if !found || cohort.Scope != "team" || cohort.TargetBranch != "same-as-job" {
				t.Fatalf("missing comparison cohort: %+v", body.Policy.Cohorts)
			}
		} else if objective.Assessment != jobpolicy.AssessmentJobPassRate || objective.Checks.PassRate == nil || objective.Checks.PassRate.MinimumPassRateBPS != 8000 || objective.Checks.PassRate.MinimumRuns != 10 {
			t.Fatalf("wrong periodic objective: %+v", objective)
		}
	}
	for _, path := range []string{"/api/teams/hypershift/jobs", "/api/teams/hypershift/policy", "/api/teams/hypershift/jobs/" + body.Jobs[0].Job.ID} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 || response.Header().Get("ETag") == "" {
			t.Fatalf("uncacheable response at %s", path)
		}
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("If-None-Match", response.Header().Get("ETag"))
		conditional := httptest.NewRecorder()
		handler.ServeHTTP(conditional, request)
		if conditional.Code != http.StatusNotModified {
			t.Fatalf("conditional status = %d", conditional.Code)
		}
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/teams/unknown/jobs", 404}, {"/api/teams/hypershift/jobs/missing", 404}, {"/api/teams/hypershift/jobs?sort=health", 422}, {"/policy", 200},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", tc.path, nil))
		if response.Code != tc.status {
			t.Fatalf("%s: status %d", tc.path, response.Code)
		}
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/openapi.json", nil))
	var spec struct {
		Paths map[string]any `json:"paths"`
	}
	json.Unmarshal(response.Body.Bytes(), &spec)
	for _, path := range []string{"/api/teams/{team}/jobs", "/api/teams/{team}/jobs/{id}", "/api/teams/{team}/policy"} {
		if spec.Paths[path] == nil {
			t.Fatalf("missing OpenAPI path %s", path)
		}
	}
	for _, field := range [][]byte{[]byte("health_objective"), []byte("job_permafailing"), []byte("correlated_failures"), []byte("insufficient_evidence")} {
		if !bytes.Contains(response.Body.Bytes(), field) {
			t.Fatalf("missing health objective schema field %s", field)
		}
	}
	// The policy API works without a health snapshot and cannot change the registry API.
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/job-registry", nil))
	var returned jobregistry.Registry
	json.Unmarshal(response.Body.Bytes(), &returned)
	a, _ := jobpolicy.Digest(registry)
	b, _ := jobpolicy.Digest(returned)
	if a != b {
		t.Fatal("policy modified registry response")
	}
}

func TestJobPolicyCommand(t *testing.T) {
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"job-policy", "--job-registry=../jobregistry/testdata/job-registry.json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var catalog jobpolicy.Catalog
	if err := json.Unmarshal(output.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	registry, _ := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err := catalog.Validate(registry); err != nil {
		t.Fatal(err)
	}
}
