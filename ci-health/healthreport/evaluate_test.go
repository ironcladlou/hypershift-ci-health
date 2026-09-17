package healthreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
)

func TestEvaluateJoinsSippyMembershipWithoutMutatingPlan(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := reportplan.Build(registry, reportplan.DefaultSelectionPolicy("main", "5.1"))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := reportplan.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	observation := &sippy.Observation{APIVersion: sippy.CurrentObservationAPIVersion, PlanDigest: digest, SourceBaseURL: "https://sippy.example.test", Releases: []string{"5.1"}, StartedAt: now.Add(-time.Minute), ObservedAt: now, Complete: true, Memberships: []sippy.ComponentReadinessMembership{{Release: "5.1", ProwJobName: "not-in-registry", Tier: sippy.JobTierStandard}}, Analyses: []sippy.JobAnalysis{}, RecentFailures: []sippy.RecentFailure{}, Collection: []sippy.CollectionResult{}}
	report, err := Evaluate(registry, plan, observation)
	if err != nil {
		t.Fatal(err)
	}
	component := report.Windows["1w"].ComponentReadinessJobs
	if len(component) != 1 || !component[0].RegistryMissing || component[0].Prow != "not-in-registry" {
		t.Fatalf("component readiness join = %+v", component)
	}
	if len(plan.Presubmits) != 69 {
		t.Fatalf("plan mutated: %d presubmits", len(plan.Presubmits))
	}
}

func TestSerializedReportIsCanonical(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := reportplan.Build(registry, reportplan.DefaultSelectionPolicy("main", "5.1"))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := reportplan.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	observation := &sippy.Observation{APIVersion: sippy.CurrentObservationAPIVersion, PlanDigest: digest, SourceBaseURL: "https://sippy.example.test", Releases: append([]string(nil), plan.Releases...), StartedAt: now.Add(-time.Minute), ObservedAt: now, Complete: true, Memberships: []sippy.ComponentReadinessMembership{}, Analyses: []sippy.JobAnalysis{}, RecentFailures: []sippy.RecentFailure{}, Collection: []sippy.CollectionResult{}}
	report, err := Evaluate(registry, plan, observation)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "health-report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, registry, plan, observation); err != nil {
		t.Fatalf("load canonical report: %v", err)
	}
	report.Releases = []string{"tampered"}
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, registry, plan, observation); err == nil {
		t.Fatal("expected tampered report to be rejected")
	}
}

func TestAnalysisResultsAreReleaseScoped(t *testing.T) {
	key50 := analysisKey{release: "5.0", jobID: "shared"}
	key51 := analysisKey{release: "5.1", jobID: "shared"}
	summaries := map[analysisKey]*analysisSummary{key50: {CurrentPassPercentage: 50}, key51: {CurrentPassPercentage: 90}}
	health := buildPeriodicHealth(key51, "shared", "shared", "shared", "5.1", "test", "", "", "", summaries, nil)
	if health.Rate == nil || *health.Rate != 90 {
		t.Fatalf("5.1 health rate = %v, want 90", health.Rate)
	}
}
