package sippy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
)

func TestCollectObservationDiscoversComponentAnalysisTargets(t *testing.T) {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/jobs":
			w.Write([]byte(`[{"name":"` + component + `","variants":["JobTier:standard"]}]`))
		case "/api/jobs/analysis":
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
}
