package jobregistry

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
)

const goldenRegistryPath = "testdata/job-registry.json"

func loadGoldenRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := LoadFile(goldenRegistryPath)
	if err != nil {
		t.Fatalf("load golden registry: %v", err)
	}
	return registry
}

func cloneRegistry(t *testing.T, registry *Registry) *Registry {
	t.Helper()
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	var clone Registry
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatalf("unmarshal registry: %v", err)
	}
	return &clone
}

func TestGoldenRegistrySnapshot(t *testing.T) {
	registry := loadGoldenRegistry(t)
	metrics := map[string]int{"jobs": len(registry.Jobs)}
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		switch job.Type {
		case "periodic":
			metrics["periodics"]++
		case "presubmit":
			metrics["presubmits"]++
		}
		if job.Presubmit != nil && len(job.Presubmit.PeriodicCounterparts) > 0 {
			metrics["presubmits with counterparts"]++
			metrics["counterparts"] += len(job.Presubmit.PeriodicCounterparts)
		}
	}
	tests := []struct {
		name string
		want int
	}{
		{"jobs", 893},
		{"periodics", 464},
		{"presubmits", 429},
		{"presubmits with counterparts", 24},
		{"counterparts", 24},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := metrics[test.name]; got != test.want {
				t.Errorf("%s = %d, want %d", test.name, got, test.want)
			}
		})
	}
}

func TestGoldenRegistryInvariants(t *testing.T) {
	registry := loadGoldenRegistry(t)
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if i > 0 && registry.Jobs[i-1].ID >= job.ID {
			t.Errorf("jobs are not strictly ordered at %q", job.ID)
		}
		if job.ID != job.Name {
			t.Errorf("job %q has different name %q", job.ID, job.Name)
		}
		assertSortedUnique(t, job.ID+" versions", job.Versions)
		assertSortedUnique(t, job.ID+" platforms", job.Platforms)
		if job.Source.Repository != releaseRepository || job.Source.Path == "" || job.Source.URL == "" {
			t.Errorf("job %q has incomplete source: %+v", job.ID, job.Source)
		}
		if job.ProwJobHistoryURL == "" {
			t.Errorf("job %q has no Prow history URL", job.ID)
		}
		if job.SippyURL != nil {
			parsed, err := url.Parse(*job.SippyURL)
			if err != nil {
				t.Errorf("job %q has invalid Sippy URL: %v", job.ID, err)
			} else if !strings.Contains(parsed.RawQuery, url.QueryEscape(job.Name)) {
				t.Errorf("job %q Sippy URL does not query its name: %s", job.ID, *job.SippyURL)
			}
		}
		if job.Presubmit != nil {
			counterpartIDs := make([]string, len(job.Presubmit.PeriodicCounterparts))
			for i := range job.Presubmit.PeriodicCounterparts {
				counterpartIDs[i] = job.Presubmit.PeriodicCounterparts[i].JobID
			}
			assertSortedUnique(t, job.ID+" periodic counterparts", counterpartIDs)
		}
	}
}

func assertSortedUnique(t *testing.T, name string, values []string) {
	t.Helper()
	if !slices.IsSorted(values) {
		t.Errorf("%s are not sorted: %v", name, values)
	}
	for i := 1; i < len(values); i++ {
		if values[i-1] == values[i] {
			t.Errorf("%s contain duplicate %q", name, values[i])
		}
	}
}

func TestGoldenRegistryJobQueries(t *testing.T) {
	registry := loadGoldenRegistry(t)
	index := registry.Index()
	tests := []struct {
		name              string
		id                string
		jobType           string
		versions          []string
		platforms         []string
		counterpartID     string
		counterpartSource PeriodicCounterpartSource
		targetBranch      string
		targetRelease     string
		testedRelease     string
	}{
		{
			name:    "main presubmit with aliased periodic",
			id:      "pull-ci-openshift-hypershift-main-e2e-aws",
			jobType: "presubmit", platforms: []string{"aws"},
			counterpartID:     "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn",
			counterpartSource: PeriodicCounterpartSourceRegistryManual,
			targetBranch:      "main", targetRelease: "5.1", testedRelease: "5.1",
		},
		{
			name:    "release presubmit with exact periodic",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-aks",
			jobType: "presubmit", versions: []string{"4.22"}, platforms: []string{"aro"},
			counterpartID:     "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks",
			counterpartSource: PeriodicCounterpartSourceRegistryManual,
			targetBranch:      "release-4.22", targetRelease: "4.22", testedRelease: "4.22",
		},
		{
			name:    "main compatibility presubmit",
			id:      "pull-ci-openshift-hypershift-main-e2e-aws-5-0",
			jobType: "presubmit", versions: []string{"5.0"}, platforms: []string{"aws"},
			targetBranch: "main", targetRelease: "5.1",
		},
		{
			name:    "release branch compatibility presubmit",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-aws-4-21",
			jobType: "presubmit", versions: []string{"4.21", "4.22"}, platforms: []string{"aws"},
			targetBranch: "release-4.22", targetRelease: "4.22",
		},
		{
			name:    "same-name jobs are not inferred",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws",
			jobType: "presubmit", versions: []string{"4.22"}, platforms: []string{"aws"},
			targetBranch: "release-4.22", targetRelease: "4.22",
		},
		{
			name:    "periodic",
			id:      "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws",
			jobType: "periodic", versions: []string{"4.22"}, platforms: []string{"aws"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := index[test.id]
			if job == nil {
				t.Fatalf("job %q not found", test.id)
			}
			if job.Type != test.jobType || !slices.Equal(job.Versions, test.versions) || !slices.Equal(job.Platforms, test.platforms) {
				t.Errorf("job metadata = type %q, versions %v, platforms %v", job.Type, job.Versions, job.Platforms)
			}
			if test.targetBranch != "" && (job.Presubmit == nil || job.Presubmit.TargetBranch != test.targetBranch || job.Presubmit.TargetRelease != test.targetRelease) {
				t.Errorf("target = %+v", job.Presubmit)
			}
			if test.counterpartID == "" {
				if job.Presubmit != nil && len(job.Presubmit.PeriodicCounterparts) != 0 {
					t.Errorf("unexpected periodic counterparts = %+v", job.Presubmit.PeriodicCounterparts)
				}
				return
			}
			if job.Presubmit == nil || len(job.Presubmit.PeriodicCounterparts) != 1 {
				t.Fatalf("periodic counterparts = %+v", job.Presubmit)
			}
			counterpart := job.Presubmit.PeriodicCounterparts[0]
			if counterpart.JobID != test.counterpartID || counterpart.TestedRelease != test.testedRelease || counterpart.Source != test.counterpartSource || counterpart.Verification != PeriodicCounterpartVerificationHuman || counterpart.Rationale == "" {
				t.Errorf("counterpart = %+v", counterpart)
			}
		})
	}
}

func TestValidateRejectsBrokenInvariants(t *testing.T) {
	golden := loadGoldenRegistry(t)
	withCounterpart := func(registry *Registry) (*Job, *PeriodicCounterpart) {
		job := registry.Index()["pull-ci-openshift-hypershift-main-e2e-aws"]
		return job, &job.Presubmit.PeriodicCounterparts[0]
	}
	tests := []struct {
		name        string
		mutate      func(*Registry)
		wantInError string
	}{
		{"API version", func(r *Registry) { r.APIVersion = "job-registry/v0" }, "unsupported job registry API version"},
		{"empty ID", func(r *Registry) { r.Jobs[0].ID = "" }, "empty ID or name"},
		{"duplicate ID", func(r *Registry) { r.Jobs[1].ID = r.Jobs[0].ID }, "duplicate job ID"},
		{"unsupported type", func(r *Registry) { r.Jobs[0].Type = "postsubmit" }, "unsupported type"},
		{"presubmit metadata missing", func(r *Registry) { r.Index()["pull-ci-openshift-hypershift-main-e2e-aws"].Presubmit = nil }, "has no presubmit configuration"},
		{"periodic has presubmit metadata", func(r *Registry) { r.Jobs[0].Presubmit = &Presubmit{} }, "has presubmit configuration"},
		{"incomplete counterpart", func(r *Registry) { _, c := withCounterpart(r); c.Rationale = "" }, "incomplete periodic counterpart"},
		{"unsupported counterpart source", func(r *Registry) { _, c := withCounterpart(r); c.Source = "heuristic" }, "unsupported source"},
		{"unsupported counterpart verification", func(r *Registry) { _, c := withCounterpart(r); c.Verification = "suggested" }, "unsupported verification"},
		{"duplicate counterpart", func(r *Registry) {
			j, c := withCounterpart(r)
			j.Presubmit.PeriodicCounterparts = append(j.Presubmit.PeriodicCounterparts, *c)
		}, "duplicate periodic counterpart"},
		{"missing periodic", func(r *Registry) { _, c := withCounterpart(r); c.JobID = "missing" }, "is not a periodic job"},
		{"release mismatch", func(r *Registry) { _, c := withCounterpart(r); c.TestedRelease = "4.22" }, "does not uniquely test release"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := cloneRegistry(t, golden)
			test.mutate(registry)
			err := registry.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, test.wantInError)
			}
		})
	}
}
