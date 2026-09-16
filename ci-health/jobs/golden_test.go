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
			t.Errorf("4.23 presubmit leaked into catalog: %s", job.ProwJobName)
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
		{"presubmit query count", len(catalog.PresubmitProwJobNames()), 69},
		{"Sippy presubmit query count", len(catalog.SippyPresubmitProwJobNames()), 11},
	}
	for _, test := range aggregates {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Errorf("got %v, want %v", test.got, test.want)
			}
		})
	}

	periodics := catalog.PeriodicProwJobNamesByRelease()
	queries := []struct {
		release string
		name    string
	}{
		{"4.22", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"},
		{"5.1", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"},
	}
	for _, query := range queries {
		t.Run(query.release+"/"+query.name, func(t *testing.T) {
			if !slices.Contains(periodics[query.release], query.name) {
				t.Errorf("periodics[%q] does not contain %q", query.release, query.name)
			}
		})
	}

	presubmits := catalog.PresubmitProwJobNames()
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

	sippyPresubmits := catalog.SippyPresubmitProwJobNames()
	sippyQueries := []struct {
		name string
		want bool
	}{
		{"pull-ci-openshift-hypershift-main-e2e-aws-5-0", true},
		{"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", false},
	}
	for _, query := range sippyQueries {
		t.Run("Sippy presubmit/"+query.name, func(t *testing.T) {
			if got := slices.Contains(sippyPresubmits, query.name); got != query.want {
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
	if len(jobs) != 1 || jobs[0].Release != "5.0" || jobs[0].ProwJobName != "kept" {
		t.Fatalf("ComponentReadinessJobs() = %+v", jobs)
	}
}

func TestGoldenRegistryBlockingJobPairingQueries(t *testing.T) {
	catalog := goldenCatalog(t)
	index := make(map[string]BlockingJobConfig, len(catalog.BlockingJobs))
	for _, job := range catalog.BlockingJobs {
		index[job.ProwJobName] = job
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
			if job.TargetRelease != test.target || got.ProwJobName != test.periodic || got.Release != test.release || got.RelationshipSource != test.source || got.RelationshipVerification != test.verification || got.RelationshipRationale == "" {
				t.Errorf("periodic pairing = %+v", got)
			}
		})
	}
}

func TestGoldenRegistryUnpairedPresubmitsRemainVisible(t *testing.T) {
	catalog := goldenCatalog(t)
	index := make(map[string]BlockingJobConfig, len(catalog.BlockingJobs))
	for _, job := range catalog.BlockingJobs {
		index[job.ProwJobName] = job
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
			if job.TargetRelease != test.targetRelease || len(job.Periodics) != 0 {
				t.Errorf("job = %+v", job)
			}
		})
	}
}
