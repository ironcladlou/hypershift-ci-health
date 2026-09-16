package jobs

import (
	"reflect"
	"slices"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

func goldenCatalog(t *testing.T) *Catalog {
	t.Helper()
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	catalog, err := NewCatalog(registry)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	return catalog
}

func TestGoldenRegistryCatalogQueries(t *testing.T) {
	catalog := goldenCatalog(t)
	for _, job := range catalog.BlockingJobs {
		if slices.Contains(job.Job.Versions, "4.23") {
			t.Errorf("4.23 presubmit leaked into catalog: %s", job.Job.Name)
		}
	}
	aggregates := []struct {
		name string
		got  any
		want any
	}{
		{"blocking job count", len(catalog.BlockingJobs), 69},
		{"payload-blocking job count", len(catalog.PayloadBlockingJobs), 37},
		{"releases", catalog.Releases(), []string{"4.14", "4.15", "4.16", "4.17", "4.18", "4.19", "4.20", "4.21", "4.22", "5.0", "5.1"}},
		{"platforms", catalog.Platforms(), []string{"aro", "aws", "azure", "gcp", "kubevirt"}},
	}
	for _, test := range aggregates {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Errorf("got %v, want %v", test.got, test.want)
			}
		})
	}

	targets := catalog.AnalysisTargets()
	seenTargets := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		key := target.Release + "\x00" + target.Job.ID
		if _, found := seenTargets[key]; found {
			t.Errorf("duplicate analysis target %q/%q", target.Release, target.Job.ID)
		}
		seenTargets[key] = struct{}{}
	}
	hasTarget := func(release, name string) bool {
		return slices.ContainsFunc(targets, func(target AnalysisTarget) bool {
			return target.Release == release && target.Job.Name == name
		})
	}
	queries := []struct {
		release string
		name    string
	}{
		{"4.22", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"},
		{"5.1", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"},
	}
	for _, query := range queries {
		t.Run(query.release+"/"+query.name, func(t *testing.T) {
			if !hasTarget(query.release, query.name) {
				t.Errorf("analysis targets do not contain %q/%q", query.release, query.name)
			}
		})
	}

	presubmits := make([]string, 0, len(catalog.BlockingJobs))
	for _, job := range catalog.BlockingJobs {
		presubmits = append(presubmits, job.Job.Name)
	}
	presubmitQueries := []struct {
		name string
		want bool
	}{
		{"pull-ci-openshift-hypershift-main-e2e-aws-5-0", true},
		{"pull-ci-openshift-hypershift-release-5.0-4-23-e2e-aws", false},
		{"pull-ci-openshift-hypershift-release-5.1-e2e-aws-5-0", false},
		{"pull-ci-openshift-hypershift-release-5.2-e2e-aws-5-0", false},
	}
	for _, query := range presubmitQueries {
		t.Run("presubmit/"+query.name, func(t *testing.T) {
			if got := slices.Contains(presubmits, query.name); got != query.want {
				t.Errorf("membership = %t, want %t", got, query.want)
			}
		})
	}

	sippyQueries := []struct {
		name string
		want bool
	}{
		{"pull-ci-openshift-hypershift-main-e2e-aws-5-0", true},
		{"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", false},
	}
	for _, query := range sippyQueries {
		t.Run("Sippy presubmit/"+query.name, func(t *testing.T) {
			if got := hasTarget("Presubmits", query.name); got != query.want {
				t.Errorf("membership = %t, want %t", got, query.want)
			}
		})
	}
}

func TestComponentReadinessExcludesUnsupportedReleases(t *testing.T) {
	catalog := goldenCatalog(t)
	jobs := catalog.ComponentReadinessJobs([]ComponentReadinessMembership{
		{Release: "5.0", ProwJobName: "kept", Tier: JobTierStandard},
		{Release: "4.23", ProwJobName: "excluded-4.23", Tier: JobTierStandard},
		{Release: "5.0", ProwJobName: "excluded-candidate", Tier: JobTierCandidate},
	})
	if len(jobs) != 1 || jobs[0].Membership.Release != "5.0" || jobs[0].Membership.ProwJobName != "kept" || jobs[0].Job != nil {
		t.Fatalf("ComponentReadinessJobs() = %+v", jobs)
	}
}

func TestGoldenRegistryBlockingJobPairingQueries(t *testing.T) {
	catalog := goldenCatalog(t)
	index := make(map[string]BlockingJobConfig, len(catalog.BlockingJobs))
	for _, job := range catalog.BlockingJobs {
		index[job.Job.Name] = job
	}
	tests := []struct {
		name         string
		presubmit    string
		periodic     string
		target       string
		release      string
		source       jobregistry.PeriodicCounterpartSource
		verification jobregistry.PeriodicCounterpartVerification
	}{
		{
			"main exact match",
			"pull-ci-openshift-hypershift-main-e2e-v2-aws",
			"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws",
			"5.1",
			"5.1",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
		{
			"release exact match",
			"pull-ci-openshift-hypershift-release-4.22-e2e-aks",
			"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks",
			"4.22",
			"4.22",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
		{
			"AWS name alias",
			"pull-ci-openshift-hypershift-main-e2e-aws",
			"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn",
			"5.1",
			"5.1",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
		{
			"KubeVirt name alias",
			"pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced",
			"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi",
			"5.1",
			"5.1",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
		{
			"conformance name alias on release branch",
			"pull-ci-openshift-hypershift-release-4.16-e2e-conformance",
			"periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance",
			"4.16",
			"4.16",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
		{
			"verified branch-derived mapping",
			"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws",
			"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws",
			"4.22",
			"4.22",
			jobregistry.PeriodicCounterpartSourceRegistryManual,
			jobregistry.PeriodicCounterpartVerificationHuman,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, found := index[test.presubmit]
			if !found {
				t.Fatalf("presubmit %q not found", test.presubmit)
			}
			if len(job.Periodics) != 1 {
				t.Fatalf("periodics = %+v", job.Periodics)
			}
			got := job.Periodics[0]
			if job.Job.Presubmit.TargetRelease != test.target || got.Job.Name != test.periodic || got.Counterpart.TestedRelease != test.release || got.Counterpart.Source != test.source || got.Counterpart.Verification != test.verification || got.Counterpart.Rationale == "" {
				t.Errorf("periodic pairing = %+v", got)
			}
		})
	}
}

func TestGoldenRegistryUnpairedPresubmitsRemainVisible(t *testing.T) {
	catalog := goldenCatalog(t)
	index := make(map[string]BlockingJobConfig, len(catalog.BlockingJobs))
	for _, job := range catalog.BlockingJobs {
		index[job.Job.Name] = job
	}
	tests := []struct {
		name          string
		presubmit     string
		targetRelease string
	}{
		{"main compatibility job", "pull-ci-openshift-hypershift-main-e2e-aws-5-0", "5.1"},
		{"release compatibility job", "pull-ci-openshift-hypershift-release-4.22-e2e-aws-4-21", "4.22"},
		{"main operator upgrade job", "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator", "5.1"},
		{"release operator upgrade job", "pull-ci-openshift-hypershift-release-4.22-e2e-aws-upgrade-hypershift-operator", "4.22"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, found := index[test.presubmit]
			if !found {
				t.Fatalf("presubmit %q not found", test.presubmit)
			}
			if job.Job.Presubmit.TargetRelease != test.targetRelease || len(job.Periodics) != 0 {
				t.Errorf("job = %+v", job)
			}
		})
	}
}
