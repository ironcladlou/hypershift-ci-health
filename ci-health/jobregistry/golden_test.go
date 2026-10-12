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
		{"presubmits with counterparts", 54},
		{"counterparts", 54},
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
			if job.Presubmit.SippyIngestion.Basis == "" {
				t.Errorf("presubmit %q has no Sippy ingestion basis", job.ID)
			}
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
		jobType           JobType
		versions          []string
		platforms         []string
		counterpartID     string
		counterpartSource PeriodicCounterpartSource
		verification      PeriodicCounterpartVerification
		targetBranch      string
		targetRelease     string
		testedRelease     string
		sippyEnabled      bool
	}{
		{
			name:    "main presubmit with aliased periodic",
			id:      "pull-ci-openshift-hypershift-main-e2e-aws",
			jobType: "presubmit", platforms: []string{"aws"},
			counterpartID:     "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn",
			counterpartSource: PeriodicCounterpartSourceRegistryManual,
			verification:      PeriodicCounterpartVerificationHuman,
			targetBranch:      "main", targetRelease: "5.1", testedRelease: "5.1",
			sippyEnabled: true,
		},
		{
			name:    "release presubmit with exact periodic",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-aks",
			jobType: "presubmit", versions: []string{"4.22"}, platforms: []string{"aro"},
			counterpartID:     "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks",
			counterpartSource: PeriodicCounterpartSourceRegistryManual,
			verification:      PeriodicCounterpartVerificationHuman,
			targetBranch:      "release-4.22", targetRelease: "4.22", testedRelease: "4.22",
		},
		{
			name:    "main compatibility presubmit",
			id:      "pull-ci-openshift-hypershift-main-e2e-aws-5-0",
			jobType: "presubmit", versions: []string{"5.0"}, platforms: []string{"aws"},
			targetBranch: "main", targetRelease: "5.1",
			sippyEnabled: true,
		},
		{
			name:    "release branch compatibility presubmit",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-aws-4-21",
			jobType: "presubmit", versions: []string{"4.21", "4.22"}, platforms: []string{"aws"},
			targetBranch: "release-4.22", targetRelease: "4.22",
		},
		{
			name:    "operator upgrade presubmit has no periodic counterpart",
			id:      "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator",
			jobType: "presubmit", platforms: []string{"aws"},
			targetBranch: "main", targetRelease: "5.1",
			sippyEnabled: true,
		},
		{
			name:    "verified branch-derived mapping",
			id:      "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws",
			jobType: "presubmit", versions: []string{"4.22"}, platforms: []string{"aws"},
			counterpartID:     "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws",
			counterpartSource: PeriodicCounterpartSourceRegistryManual,
			verification:      PeriodicCounterpartVerificationHuman,
			targetBranch:      "release-4.22", targetRelease: "4.22", testedRelease: "4.22",
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
			if job.Presubmit != nil && (job.Presubmit.SippyIngestion.Enabled != test.sippyEnabled || job.Presubmit.SippyIngestion.Basis == "") {
				t.Errorf("Sippy ingestion = %+v, want enabled=%t with a basis", job.Presubmit.SippyIngestion, test.sippyEnabled)
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
			if counterpart.JobID != test.counterpartID || counterpart.TestedRelease != test.testedRelease || counterpart.Source != test.counterpartSource || counterpart.Verification != test.verification || counterpart.Rationale == "" {
				t.Errorf("counterpart = %+v", counterpart)
			}
		})
	}
}

func TestOptionalFuturePeriodicCounterparts(t *testing.T) {
	registry := loadGoldenRegistry(t)
	// Exercise generation as well as the serialized fixture. Optionality must
	// not prevent registry relationships from being published.
	if err := populatePeriodicCounterparts(registry, defaultPresubmitPolicy); err != nil {
		t.Fatal(err)
	}
	index := registry.Index()
	for _, test := range []struct {
		presubmit, periodic string
	}{
		{"e2e-conformance", "e2e-aws-ovn-conformance"},
		{"e2e-conformance-fips", "e2e-aws-ovn-conformance-fips"},
		{"e2e-openstack-aws-csi-cinder", "e2e-openstack-aws-csi-cinder"},
		{"e2e-azure-kubevirt-ovn", "e2e-azure-kubevirt-ovn"},
	} {
		t.Run(test.presubmit, func(t *testing.T) {
			job := index["pull-ci-openshift-hypershift-main-"+test.presubmit]
			if job == nil || job.Presubmit == nil || job.Presubmit.Required {
				t.Fatal("expected an optional presubmit")
			}
			counterparts := job.Presubmit.PeriodicCounterparts
			if len(counterparts) != 1 {
				t.Fatalf("counterparts = %+v", counterparts)
			}
			c := counterparts[0]
			if c.JobID != "periodic-ci-openshift-hypershift-release-5.1-periodics-"+test.periodic || c.TestedRelease != "5.1" || c.Source != PeriodicCounterpartSourceRegistryManual || c.Verification != PeriodicCounterpartVerificationHuman || c.Rationale == "" || c.Notes == "" {
				t.Fatalf("unexpected verified mapping: %+v", c)
			}
		})
	}
	for _, scenario := range []string{"e2e-kubevirt-aws-ovn", "e2e-kubevirt-azure-ovn", "e2e-aws-techpreview", "e2e-aws-major-version-upgrade", "e2e-agent-connected-ovn-ipv4-metal-backuprestore", "e2e-kubevirt-metal-ovn-backuprestore", "reqserving-e2e-aws"} {
		job := index["pull-ci-openshift-hypershift-main-"+scenario]
		if job == nil || job.Presubmit == nil {
			t.Fatalf("missing presubmit %q", scenario)
		}
		if len(job.Presubmit.PeriodicCounterparts) != 0 {
			t.Errorf("different scenarios or older releases must not be paired with %q", scenario)
		}
	}
	kubevirt := index["pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced"].Presubmit
	if len(kubevirt.PeriodicCounterparts) != 1 || kubevirt.PeriodicCounterparts[0].Verification != PeriodicCounterpartVerificationHuman {
		t.Fatalf("existing KubeVirt mapping changed: %+v", kubevirt)
	}
	legacyJSON, err := json.Marshal(kubevirt.PeriodicCounterparts[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyJSON), `"notes"`) {
		t.Fatal("existing mappings must omit notes until they are recorded")
	}
	for _, test := range []struct{ scenario, nuance string }{
		{"e2e-aws-ovn-conformance-ccm", "monthly"},
		{"e2e-aws-ovn-conformance-techpreview", "NLB"},
		{"e2e-azure-aks-ovn-conformance-fips", "marketplace"},
		{"e2e-azure-kubevirt-ovn", "stable-4.20"},
		{"e2e-openstack-aws", "six-hour"},
		{"e2e-v2-aws-backuprestore", "weekly"},
	} {
		c := index["pull-ci-openshift-hypershift-main-"+test.scenario].Presubmit.PeriodicCounterparts[0]
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		var decoded PeriodicCounterpart
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Notes != c.Notes || !strings.Contains(decoded.Notes, test.nuance) {
			t.Errorf("%s notes lost their scenario nuance: %q", test.scenario, decoded.Notes)
		}
	}
}

func TestVerifiedCounterpartsWithMissingEndpoints(t *testing.T) {
	registry := loadGoldenRegistry(t)
	const presubmitID = "pull-ci-openshift-hypershift-main-e2e-aks-managed-hsm"
	const periodicID = "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks-managed-hsm"
	if err := populatePeriodicCounterparts(registry, defaultPresubmitPolicy); err != nil {
		t.Fatal(err)
	}
	if len(registry.Index()[presubmitID].Presubmit.PeriodicCounterparts) != 0 {
		t.Fatal("mapping must be absent when its periodic is absent from the source snapshot")
	}
	periodic := *registry.Index()["periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks"]
	periodic.ID, periodic.Name = periodicID, periodicID
	registry.Jobs = append(registry.Jobs, periodic)
	if err := populatePeriodicCounterparts(registry, defaultPresubmitPolicy); err != nil {
		t.Fatal(err)
	}
	cs := registry.Index()[presubmitID].Presubmit.PeriodicCounterparts
	if len(cs) != 1 || cs[0].JobID != periodicID || cs[0].Verification != PeriodicCounterpartVerificationHuman || cs[0].Notes == "" {
		t.Fatalf("mapping must become available when both jobs exist: %+v", cs)
	}
	registry.Jobs = slices.DeleteFunc(registry.Jobs, func(job Job) bool { return job.ID == presubmitID })
	if err := populatePeriodicCounterparts(registry, defaultPresubmitPolicy); err != nil {
		t.Fatalf("absent verified presubmit: %v", err)
	}
}

func TestSippyIngestionReleaseAllowlist(t *testing.T) {
	golden := loadGoldenRegistry(t)
	tests := []struct {
		name      string
		allowlist []string
		jobID     string
		enabled   bool
	}{
		{"main is enabled independently", nil, "pull-ci-openshift-hypershift-main-e2e-aws", true},
		{"release branch is disabled by default", nil, "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", false},
		{"allowlisted release branch is enabled", []string{"4.22"}, "pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", true},
		{"other release branch remains disabled", []string{"4.22"}, "pull-ci-openshift-hypershift-release-4.21-e2e-aws", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := cloneRegistry(t, golden)
			policy := defaultPresubmitPolicy
			policy.sippyReleaseBranchAllowlist = append([]string(nil), test.allowlist...)
			populateSippyIngestion(registry, policy)
			job := registry.Index()[test.jobID]
			if job == nil || job.Presubmit == nil {
				t.Fatalf("presubmit %q not found", test.jobID)
			}
			if job.Presubmit.SippyIngestion.Enabled != test.enabled || job.Presubmit.SippyIngestion.Basis == "" {
				t.Errorf("Sippy ingestion = %+v, want enabled=%t with a basis", job.Presubmit.SippyIngestion, test.enabled)
			}
			if err := registry.Validate(); err != nil {
				t.Errorf("Validate() = %v", err)
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
		{"duplicate ID", func(r *Registry) { r.Jobs[1].ID = r.Jobs[0].ID }, "duplicate job ID"},
		{"unsupported type", func(r *Registry) { r.Jobs[0].Type = "postsubmit" }, "unsupported type"},
		{"presubmit metadata missing", func(r *Registry) { r.Index()["pull-ci-openshift-hypershift-main-e2e-aws"].Presubmit = nil }, "has no presubmit configuration"},
		{"periodic has presubmit metadata", func(r *Registry) { r.Jobs[0].Presubmit = &Presubmit{} }, "has presubmit configuration"},
		{"incomplete counterpart", func(r *Registry) { _, c := withCounterpart(r); c.Rationale = "" }, "incomplete periodic counterpart"},
		{"unsupported counterpart source", func(r *Registry) { _, c := withCounterpart(r); c.Source = "heuristic" }, "unsupported source"},
		{"unsupported counterpart verification", func(r *Registry) { _, c := withCounterpart(r); c.Verification = "suggested" }, "unsupported verification"},
		{"missing Sippy ingestion basis", func(r *Registry) {
			r.Index()["pull-ci-openshift-hypershift-main-e2e-aws"].Presubmit.SippyIngestion.Basis = ""
		}, "no Sippy ingestion basis"},
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
