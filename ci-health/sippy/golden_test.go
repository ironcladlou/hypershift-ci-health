package sippy

import (
	"slices"
	"testing"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
)

func TestGoldenRegistryPresubmitHealthQueries(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	catalog, err := jobs.NewCatalog(registry, jobs.CatalogOptions{DevelopmentBranch: "main", DevelopmentRelease: "5.1"})
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	window := transformWindow(
		&rawData{analyses: map[analysisKey]*SippyJobAnalysisResponse{}},
		"1w",
		time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		catalog,
	)
	index := make(map[string]JobHealth, len(window.Jobs))
	for _, job := range window.Jobs {
		index[job.Prow] = job
	}
	tests := []struct {
		name            string
		presubmit       string
		targetBranch    string
		targetRelease   string
		role            string
		periodic        string
		periodicRelease string
		source          string
		verification    string
	}{
		{
			"main compatibility job is in Future",
			"pull-ci-openshift-hypershift-main-e2e-aws-5-0",
			"main", "5.1", "future",
			"", "",
			"", "",
		},
		{
			"release compatibility job is in target release",
			"pull-ci-openshift-hypershift-release-4.22-e2e-aws-4-21",
			"release-4.22", "4.22", "n-2",
			"", "",
			"", "",
		},
		{
			"human-verified mapping is exposed",
			"pull-ci-openshift-hypershift-main-e2e-aws",
			"main", "5.1", "future",
			"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn", "5.1",
			"registry-manual", "human-verified",
		},
		{
			"verified branch-derived mapping is exposed",
			"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws",
			"release-4.22", "4.22", "n-2",
			"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws", "4.22",
			"registry-manual", "human-verified",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, found := index[test.presubmit]
			if !found {
				t.Fatalf("presubmit %q not found", test.presubmit)
			}
			if job.TargetBranch != test.targetBranch || job.TargetRelease != test.targetRelease || job.Role != test.role {
				t.Errorf("target/role = %s, %s, %s", job.TargetBranch, job.TargetRelease, job.Role)
			}
			if test.periodic == "" && len(job.Periodics) != 0 {
				t.Errorf("unexpected periodics = %+v", job.Periodics)
			}
			if test.periodic != "" && (len(job.Periodics) != 1 || job.Periodics[0].Prow != test.periodic || job.Periodics[0].Release != test.periodicRelease || job.Periodics[0].RelationshipSource != test.source || job.Periodics[0].RelationshipVerification != test.verification) {
				t.Errorf("periodics = %+v", job.Periodics)
			}
		})
	}

	presubmit := index["pull-ci-openshift-hypershift-main-e2e-aws"]
	if presubmit.Name != "e2e-aws" || !slices.Equal(presubmit.Platforms, []string{"aws"}) || len(presubmit.Periodics) == 0 || presubmit.Periodics[0].RelationshipRationale == "" {
		t.Errorf("presubmit registry projection = %+v", presubmit)
	}

	var payload *PayloadBlockingJobHealth
	for i := range window.PayloadBlockingJobs {
		candidate := &window.PayloadBlockingJobs[i]
		if candidate.Prow == "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks" && candidate.Release == "5.1" {
			payload = candidate
			break
		}
	}
	if payload == nil {
		t.Fatal("5.1 e2e-aks payload blocker not found")
	}
	if payload.Name != "e2e-aks" || !slices.Equal(payload.Platforms, []string{"aro"}) || len(payload.Participations) == 0 {
		t.Errorf("payload registry projection = %+v", payload)
	}
}

func TestAnalysisResultsAreReleaseScoped(t *testing.T) {
	key50 := analysisKey{release: "5.0", jobID: "shared"}
	key51 := analysisKey{release: "5.1", jobID: "shared"}
	summaries := map[analysisKey]*SippyJob{
		key50: {CurrentPassPercentage: 50},
		key51: {CurrentPassPercentage: 90},
	}
	health := buildPeriodicHealth(key51, "shared", "shared", "shared", "5.1", "test", "", "", "", summaries, nil)
	if health.Rate == nil || *health.Rate != 90 {
		t.Fatalf("5.1 health rate = %v, want 90", health.Rate)
	}
}
