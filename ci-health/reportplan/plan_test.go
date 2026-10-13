package reportplan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

func goldenPlan(t *testing.T) (*jobregistry.Registry, *Plan) {
	t.Helper()
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	plan, err := Build(registry, DefaultSelectionPolicy("main", "5.1"))
	if err != nil {
		t.Fatalf("build report plan: %v", err)
	}
	return registry, plan
}

func TestSerializedPlanIsCanonical(t *testing.T) {
	registry, plan := goldenPlan(t)
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report-plan.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, registry); err != nil {
		t.Fatalf("load canonical plan: %v", err)
	}
	plan.Releases = []string{"tampered"}
	data, err = json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, registry); err == nil {
		t.Fatal("expected tampered plan to be rejected")
	}
}

func TestGoldenPlan(t *testing.T) {
	registry, plan := goldenPlan(t)
	if plan.APIVersion != CurrentAPIVersion || plan.RegistryDigest == "" {
		t.Fatalf("missing plan identity: %+v", plan)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"presubmit count", len(plan.Presubmits), 99},
		{"payload count", len(plan.PayloadJobs), 37},
		{"releases", plan.Releases, []string{"4.14", "4.15", "4.16", "4.17", "4.18", "4.19", "4.20", "4.21", "4.22", "5.0", "5.1"}},
		{"platforms", plan.Platforms, []string{"agent", "aro", "aws", "azure", "baremetal", "gcp", "kubevirt", "openstack"}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !reflect.DeepEqual(check.got, check.want) {
				t.Errorf("got %v, want %v", check.got, check.want)
			}
		})
	}
	index := registry.Index()
	optional := 0
	for _, entry := range plan.Presubmits {
		job := index[entry.JobID]
		if !job.Presubmit.Required {
			optional++
			if job.Presubmit.TargetBranch != "main" || job.Presubmit.TargetRelease != "5.1" {
				t.Errorf("optional presubmit escaped development scope: %s", job.ID)
			}
		}
	}
	if optional != 30 {
		t.Errorf("optional presubmits = %d, want 30", optional)
	}
	if !slices.ContainsFunc(plan.Presubmits, func(entry Presubmit) bool {
		return index[entry.JobID].Name == "pull-ci-openshift-hypershift-main-e2e-aws-5-0" && entry.Role == "future"
	}) {
		t.Error("main compatibility presubmit missing")
	}
	if !slices.ContainsFunc(plan.AnalysisTargets, func(target AnalysisTarget) bool {
		return target.Release == "5.1" && index[target.JobID].Name == "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"
	}) {
		t.Error("expected periodic analysis target missing")
	}
}

func TestPlanRejectsDifferentRegistry(t *testing.T) {
	registry, plan := goldenPlan(t)
	registry.Jobs[0].Context += "-changed"
	if err := plan.Validate(registry); err == nil {
		t.Fatal("expected registry digest mismatch")
	}
}

func TestHypershiftPayloadSelection(t *testing.T) {
	participation := func(stream, release string, role jobregistry.ReleaseControllerRole, eol bool) jobregistry.ReleaseControllerParticipation {
		return jobregistry.ReleaseControllerParticipation{Stream: jobregistry.ReleaseControllerStream{Name: stream, Release: release, EndOfLife: eol}, Verification: jobregistry.ReleaseControllerVerification{Role: role}}
	}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion}
	for _, tc := range []struct {
		id       string
		versions []string
		stream   string
		release  string
		role     jobregistry.ReleaseControllerRole
		eol      bool
	}{
		{"blocking", []string{"5.1"}, "hypershift", "", jobregistry.ReleaseControllerRoleBlocking, false},
		{"informing", []string{"4.20"}, "hypershift", "", jobregistry.ReleaseControllerRoleInforming, false},
		{"async", []string{"4.21"}, "hypershift", "", jobregistry.ReleaseControllerRoleAsync, false},
		{"disabled", []string{"4.22"}, "hypershift", "", jobregistry.ReleaseControllerRoleDisabled, false},
		{"end-of-life", []string{"4.22"}, "hypershift", "", jobregistry.ReleaseControllerRoleBlocking, true},
		{"unsupported", []string{"4.13", "4.23", "5.2"}, "hypershift", "", jobregistry.ReleaseControllerRoleBlocking, false},
		{"no-version", nil, "hypershift", "", jobregistry.ReleaseControllerRoleBlocking, false},
		{"other-unversioned", []string{"4.22"}, "other", "", jobregistry.ReleaseControllerRoleBlocking, false},
		{"ocp-informing", []string{"4.22"}, "4.22.0-0.nightly", "4.22", jobregistry.ReleaseControllerRoleInforming, false},
		{"dual-stream", []string{"4.22", "4.22"}, "hypershift", "", jobregistry.ReleaseControllerRoleInforming, false},
	} {
		registry.Jobs = append(registry.Jobs, jobregistry.Job{ID: tc.id, Name: tc.id, Type: jobregistry.JobTypePeriodic, Versions: tc.versions, ReleaseController: []jobregistry.ReleaseControllerParticipation{participation(tc.stream, tc.release, tc.role, tc.eol)}})
	}
	dual := registry.Index()["dual-stream"]
	dual.ReleaseController = append(dual.ReleaseController, participation("4.22.0-0.nightly", "4.22", jobregistry.ReleaseControllerRoleBlocking, false))
	plan, err := Build(registry, DefaultSelectionPolicy("main", "5.1"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"blocking": "5.1", "informing": "4.20", "async": "4.21", "dual-stream": "4.22"}
	if len(plan.PayloadJobs) != len(want) || len(plan.AnalysisTargets) != len(want) {
		t.Fatalf("unexpected payload or collection selection: payload=%+v targets=%+v", plan.PayloadJobs, plan.AnalysisTargets)
	}
	for _, entry := range plan.PayloadJobs {
		if want[entry.JobID] != entry.Release || !slices.Contains(plan.AnalysisTargets, AnalysisTarget{Release: entry.Release, JobID: entry.JobID}) {
			t.Errorf("wrong tested release or missing analysis: %+v", entry)
		}
		if entry.JobID == "dual-stream" && len(entry.Participations) != 2 {
			t.Errorf("overlapping streams must share one row: %+v", entry)
		}
		for _, item := range entry.Participations {
			if item.Stream.Name == "hypershift" && item.Stream.Release != "" {
				t.Error("unversioned stream metadata was rewritten")
			}
		}
	}
}

func TestFutureCollectionIncludesJobsOutsideDashboard(t *testing.T) {
	registry, plan := goldenPlan(t)
	index := registry.Index()
	for _, target := range []AnalysisTarget{
		{Release: "Presubmits", JobID: "pull-ci-openshift-hypershift-main-e2e-aws-metrics"},
		{Release: "Presubmits", JobID: "pull-ci-openshift-hypershift-main-e2e-azure-kubevirt-ovn"},
		{Release: "5.1", JobID: "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-powervs-ovn"},
		{Release: "5.1", JobID: "periodic-ci-openshift-hypershift-release-5.1-periodics-azure-perf-azure-self-managed-performance"},
	} {
		if !slices.Contains(plan.AnalysisTargets, target) {
			t.Errorf("missing collection target: %+v", target)
		}
		if index[target.JobID].Type == jobregistry.JobTypePeriodic && (slices.ContainsFunc(plan.Presubmits, func(entry Presubmit) bool { return entry.JobID == target.JobID }) || slices.ContainsFunc(plan.PayloadJobs, func(entry PayloadJob) bool { return entry.JobID == target.JobID })) {
			t.Errorf("collection-only periodic became a dashboard row: %s", target.JobID)
		}
	}
	seen := map[AnalysisTarget]bool{}
	for _, target := range plan.AnalysisTargets {
		if seen[target] {
			t.Errorf("duplicate collection target: %+v", target)
		}
		seen[target] = true
		job := index[target.JobID]
		if target.Release == "Presubmits" && job.Presubmit.TargetBranch != "main" {
			t.Errorf("release-branch presubmit unexpectedly added to collection: %s", job.ID)
		}
	}
}

func TestFutureCollectionUsesPolicyAndIngestionScope(t *testing.T) {
	newPresubmit := func(id, branch, release string, framework jobregistry.E2EFramework, enabled bool) jobregistry.Job {
		return jobregistry.Job{ID: id, Name: id, Type: jobregistry.JobTypePresubmit, E2EFramework: framework, Presubmit: &jobregistry.Presubmit{TargetBranch: branch, TargetRelease: release, SippyIngestion: jobregistry.SippyIngestion{Enabled: enabled, Basis: "test ingestion policy"}}}
	}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{
		newPresubmit("enabled-future", "development", "6.2", jobregistry.E2EFrameworkV1, true),
		newPresubmit("disabled-future", "development", "6.2", jobregistry.E2EFrameworkV1, false),
		newPresubmit("non-e2e-future", "development", "6.2", jobregistry.E2EFrameworkNone, true),
		newPresubmit("other-branch", "main", "6.2", jobregistry.E2EFrameworkV1, true),
		newPresubmit("other-release", "development", "6.1", jobregistry.E2EFrameworkV1, true),
		{ID: "future-performance", Name: "future-performance", Type: jobregistry.JobTypePeriodic, E2EFramework: jobregistry.E2EFrameworkNone, Versions: []string{"6.2"}},
		{ID: "older-periodic", Name: "older-periodic", Type: jobregistry.JobTypePeriodic, Versions: []string{"6.1"}},
	}}
	policy := DefaultSelectionPolicy("development", "6.2")
	plan, err := Build(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	want := []AnalysisTarget{{Release: "6.2", JobID: "future-performance"}, {Release: "Presubmits", JobID: "enabled-future"}}
	if !reflect.DeepEqual(plan.AnalysisTargets, want) {
		t.Fatalf("collection targets = %+v, want %+v", plan.AnalysisTargets, want)
	}
	if len(plan.Presubmits) != 2 || len(plan.PayloadJobs) != 0 || !slices.Equal(plan.Releases, []string{"6.2"}) {
		t.Fatalf("only future E2E presubmits should be displayed: %+v", plan)
	}
	for _, test := range []struct {
		name   string
		policy SelectionPolicy
	}{
		{"excluded development release", SelectionPolicy{DevelopmentBranch: "development", DevelopmentRelease: "6.2", MinimumRelease: "4.14", MaximumRelease: "6.2", ExcludedReleases: []string{"6.2"}}},
		{"development release outside range", SelectionPolicy{DevelopmentBranch: "development", DevelopmentRelease: "6.2", MinimumRelease: "4.14", MaximumRelease: "6.1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := Build(registry, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.AnalysisTargets) != 0 {
				t.Fatalf("unsupported future release was collected: %+v", plan.AnalysisTargets)
			}
		})
	}
}
