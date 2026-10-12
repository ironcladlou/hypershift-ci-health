package sippy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
)

func TestCollectObservationRejectsFailedRequests(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := reportplan.RegistryDigest(registry)
	if err != nil {
		t.Fatal(err)
	}
	plan := &reportplan.Plan{APIVersion: reportplan.CurrentAPIVersion, RegistryDigest: digest, Policy: reportplan.DefaultSelectionPolicy("main", "5.1"), Releases: []string{"5.1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/jobs" {
			http.Error(w, "membership unavailable", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"rows":[]}`))
	}))
	defer server.Close()
	observation, status, err := CollectObservation(context.Background(), &Client{BaseURL: server.URL, HTTPClient: server.Client()}, registry, plan)
	if err == nil || !strings.Contains(err.Error(), "1/2 requests failed") {
		t.Fatalf("expected a collection failure, got %v", err)
	}
	if observation == nil || observation.Complete || status.Failed != 1 {
		t.Fatalf("failed collection must retain diagnostics: observation=%+v status=%+v", observation, status)
	}
}

func TestCollectObservationCollectsTargetsOutsideDashboard(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	registryDigest, err := reportplan.RegistryDigest(registry)
	if err != nil {
		t.Fatal(err)
	}
	plan := &reportplan.Plan{APIVersion: reportplan.CurrentAPIVersion, RegistryDigest: registryDigest, Policy: reportplan.DefaultSelectionPolicy("main", "5.1"), Releases: []string{"5.1"}, Platforms: []string{}, Presubmits: []reportplan.Presubmit{}, PayloadJobs: []reportplan.PayloadJob{}, AnalysisTargets: []reportplan.AnalysisTarget{}}
	const component = "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"
	const optional = "pull-ci-openshift-hypershift-main-e2e-aws-metrics"
	const unpairedPeriodic = "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-powervs-ovn"
	plan.AnalysisTargets = []reportplan.AnalysisTarget{
		{Release: "Presubmits", JobID: optional},
		{Release: "5.1", JobID: unpairedPeriodic},
	}
	var mu sync.Mutex
	counts := map[AnalysisTarget]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/jobs":
			w.Write([]byte(`[{"name":"` + component + `","variants":["JobTier:standard"]},{"name":"` + unpairedPeriodic + `","variants":["JobTier:standard"]}]`))
		case "/api/jobs/analysis":
			var queryFilter filter
			if err := json.Unmarshal([]byte(r.URL.Query().Get("filter")), &queryFilter); err != nil || len(queryFilter.Items) != 1 {
				t.Errorf("unexpected analysis filter: %s", r.URL.Query().Get("filter"))
				http.Error(w, "invalid filter", http.StatusBadRequest)
				return
			}
			mu.Lock()
			counts[AnalysisTarget{Release: r.URL.Query().Get("release"), ProwJobName: queryFilter.Items[0].Value}]++
			mu.Unlock()
			w.Write([]byte(`{"by_period":{}}`))
		case "/api/tests/recent_failures":
			w.Write([]byte(`{"rows":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	observation, status, err := CollectObservation(context.Background(), &Client{BaseURL: server.URL, HTTPClient: server.Client()}, registry, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Complete || status.Failed != 0 {
		t.Fatalf("collection status = %+v", status)
	}
	found := false
	for _, analysis := range observation.Analyses {
		if analysis.Target.Release == "5.1" && analysis.Target.ProwJobName == component {
			found = true
		}
	}
	if !found {
		t.Fatalf("component analysis target was not collected: %+v", observation.Analyses)
	}
	for _, target := range []AnalysisTarget{
		{Release: "Presubmits", ProwJobName: optional},
		{Release: "5.1", ProwJobName: unpairedPeriodic},
		{Release: "5.1", ProwJobName: component},
	} {
		if counts[target] != 1 {
			t.Errorf("analysis requests for %+v = %d, want 1", target, counts[target])
		}
	}
	if len(observation.Analyses) != 3 {
		t.Fatalf("expected both planned jobs and the discovered component job: %+v", observation.Analyses)
	}
}
