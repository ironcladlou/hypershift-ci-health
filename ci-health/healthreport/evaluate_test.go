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
	for _, window := range []string{"24h", "48h"} {
		data := report.Windows[window]
		if data == nil || len(data.SparklineSlots) != 24 || len(data.ComponentReadinessJobs) != 1 {
			t.Fatalf("%s window = %+v", window, data)
		}
	}
}

func TestShortWindowAnalysis(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		window string
		hours  int
		step   int
	}{
		{window: "24h", hours: 24, step: 1},
		{window: "48h", hours: 48, step: 2},
	} {
		t.Run(tc.window, func(t *testing.T) {
			analysis := &analysisData{ByPeriod: map[string]sippy.AnalysisPeriod{}}
			add := func(hoursAgo, runs, passes int) {
				period := now.Add(-time.Duration(hoursAgo) * time.Hour).Format("2006-01-02 15:00")
				analysis.ByPeriod[period] = sippy.AnalysisPeriod{TotalRuns: runs, ResultCount: map[string]int{"S": passes, "F": runs - passes}}
			}
			add(1, 4, 3)
			add(tc.hours, 2, 0) // The current period includes its start boundary.
			add(tc.hours+1, 4, 4)
			add(2*tc.hours, 2, 2) // The previous period includes its start boundary.
			add(2*tc.hours+1, 100, 0)
			summary := summarizeAnalysis(analysis, tc.window, now)
			if summary.CurrentRuns != 6 || summary.CurrentFails != 3 || summary.PreviousRuns != 6 || summary.CurrentPassPercentage != 50 || summary.PreviousPassPercentage != 100 || summary.NetImprovement != -50 {
				t.Fatalf("summary = %+v", summary)
			}

			win := WindowConfigs[tc.window]
			slots := slotKeys(now, win)
			if len(slots) != 24 {
				t.Fatalf("slot count = %d, want 24", len(slots))
			}
			for index, key := range slots {
				want := now.Add(-time.Duration((23-index)*tc.step) * time.Hour).Format("2006-01-02 15:00")
				if key != want {
					t.Fatalf("slot %d = %s, want %s", index, key, want)
				}
			}
			sparkline := orderedSparkline(analysisSparkline(analysis, win, now), slots)
			var runs, passes int
			for _, slot := range sparkline {
				if slot != nil {
					runs += slot.TotalRuns
					passes += slot.Passes
				}
			}
			if runs != 4 || passes != 3 {
				t.Fatalf("sparkline runs = %d, passes = %d; want 4, 3", runs, passes)
			}
		})
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
	health := buildPeriodicHealth(key51, "shared", "shared", "shared", "5.1", "test", "", "", "", "", summaries, nil, nil)
	if health.Rate == nil || *health.Rate != 90 {
		t.Fatalf("5.1 health rate = %v, want 90", health.Rate)
	}
}

func TestSparklineSlotUsesCompactTuple(t *testing.T) {
	data, err := json.Marshal(SparklineSlot{TotalRuns: 10, Passes: 7, TestFailures: 2, InfraFailures: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[10,7,2,1]" {
		t.Fatalf("sparkline JSON = %s", data)
	}
	var slot SparklineSlot
	if err := json.Unmarshal(data, &slot); err != nil {
		t.Fatal(err)
	}
	if slot.TotalRuns != 10 || slot.Passes != 7 || slot.TestFailures != 2 || slot.InfraFailures != 1 {
		t.Fatalf("sparkline round trip = %+v", slot)
	}
}
