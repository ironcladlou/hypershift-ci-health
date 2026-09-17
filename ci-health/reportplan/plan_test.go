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
		{"presubmit count", len(plan.Presubmits), 69},
		{"payload count", len(plan.PayloadJobs), 37},
		{"releases", plan.Releases, []string{"4.14", "4.15", "4.16", "4.17", "4.18", "4.19", "4.20", "4.21", "4.22", "5.0", "5.1"}},
		{"platforms", plan.Platforms, []string{"aro", "aws", "azure", "gcp", "kubevirt"}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !reflect.DeepEqual(check.got, check.want) {
				t.Errorf("got %v, want %v", check.got, check.want)
			}
		})
	}
	index := registry.Index()
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
