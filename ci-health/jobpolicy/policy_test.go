package jobpolicy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/releaseconfig"
)

func testJob(id, release, cloud string) jobregistry.Job {
	job := jobregistry.Job{ID: id, Name: id, Type: jobregistry.JobTypePeriodic, Versions: []string{}, Platforms: []string{}, ReleaseController: []jobregistry.ReleaseControllerParticipation{}}
	if release != "" {
		job.Versions = []string{release}
	}
	if cloud != "" {
		job.Platforms = []string{cloud}
	}
	return job
}

func participant(job jobregistry.Job, role jobregistry.ReleaseControllerRole, eol bool) jobregistry.Job {
	job.ReleaseController = []jobregistry.ReleaseControllerParticipation{{Stream: jobregistry.ReleaseControllerStream{Name: "hypershift", EndOfLife: eol}, Verification: jobregistry.ReleaseControllerVerification{Role: role}}}
	return job
}

func passRateObjective(window string, rate, runs int) HealthObjective {
	return HealthObjective{Assessment: AssessmentJobPassRate, Objective: "pass-rate-at-least-threshold", Checks: HealthChecks{PassRate: &PassRateCheck{Window: window, MinimumPassRateBPS: rate, MinimumRuns: runs, Denominator: "completed-non-aborted-runs"}}, InsufficientEvidence: "indeterminate"}
}

func TestRankingAndDeterminism(t *testing.T) {
	policy, err := DefaultPolicy()
	if err != nil {
		t.Fatal(err)
	}
	policy.DevelopmentRelease = "100.1"
	required := testJob("required", "99.1", "aws")
	required.Type = jobregistry.JobTypePresubmit
	required.Presubmit = &jobregistry.Presubmit{Required: true, AlwaysRun: true, RunIfChanged: "pkg/", TargetRelease: "100.1", SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
	conditional := required
	conditional.ID = "conditional"
	conditional.Name = "conditional"
	copyPresubmit := *required.Presubmit
	conditional.Presubmit = &copyPresubmit
	conditional.Presubmit.AlwaysRun = false
	multi := testJob("multi", "4.9", "aro")
	multi.Versions = []string{"4.9", "4.10"}
	multi.Platforms = []string{"gcp", "aro"}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{
		testJob("unknown", "", ""), testJob("z-tie", "4.10", "aws"), testJob("a-tie", "4.10", "aws"),
		testJob("older-aws", "4.9", "aws"), testJob("newer-gcp", "4.11", "gcp"), multi, conditional, required,
		participant(testJob("release", "4.1", "gcp"), jobregistry.ReleaseControllerRoleInforming, false),
	}}
	before, _ := json.Marshal(registry)
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(registry)
	if string(before) != string(after) {
		t.Fatal("evaluation mutated the registry")
	}
	ids := []string{}
	for i, entry := range catalog.Jobs {
		ids = append(ids, entry.JobID)
		if entry.TeamMetadata.Importance.Rank != i+1 {
			t.Fatal("rank is not global position")
		}
	}
	want := []string{"release", "required", "conditional", "newer-gcp", "a-tie", "z-tie", "multi", "older-aws", "unknown"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	for _, entry := range catalog.Jobs {
		if entry.JobID == "multi" && (entry.TeamMetadata.Importance.ResolvedRelease != "4.10" || entry.TeamMetadata.Importance.ResolvedCloud != "azure") {
			t.Fatalf("multi-valued resolution: %+v", entry)
		}
		if entry.JobID == "required" && entry.TeamMetadata.Importance.ResolvedRelease != "100.1" {
			t.Fatal("presubmit ranking did not use its target release")
		}
	}
	slices.Reverse(registry.Jobs)
	shuffled, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	// Registry provenance changes with serialization order; evaluated metadata must not.
	if !reflect.DeepEqual(catalog.Jobs, shuffled.Jobs) {
		t.Fatal("input order changed policy output")
	}
	repeated, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Digest(shuffled)
	b, _ := Digest(repeated)
	if a != b {
		t.Fatal("identical inputs produced different snapshot digests")
	}
}

func TestParticipationAndFallbacks(t *testing.T) {
	policy, _ := DefaultPolicy()
	for _, tc := range []struct {
		name   string
		role   jobregistry.ReleaseControllerRole
		eol    bool
		stream string
		kind   jobregistry.ReleaseControllerStreamKind
		group  string
	}{
		{"blocking", jobregistry.ReleaseControllerRoleBlocking, false, "hypershift", "", "hypershift-release-controller"},
		{"async", jobregistry.ReleaseControllerRoleAsync, false, "hypershift", "", "hypershift-release-controller"},
		{"disabled", jobregistry.ReleaseControllerRoleDisabled, false, "hypershift", "", "other"},
		{"end-of-life", jobregistry.ReleaseControllerRoleBlocking, true, "hypershift", "", "other"},
		{"openshift-ci", jobregistry.ReleaseControllerRoleBlocking, false, "5.1.0-0.ci", jobregistry.ReleaseControllerStreamKindCI, "openshift-ci-release-controller"},
		{"openshift-nightly", jobregistry.ReleaseControllerRoleInforming, false, "5.1.0-0.nightly", jobregistry.ReleaseControllerStreamKindNightly, "openshift-nightly-release-controller"},
		{"multi-arch-nightly", jobregistry.ReleaseControllerRoleAsync, false, "5.1.0-0.nightly-multi", jobregistry.ReleaseControllerStreamKindNightly, "openshift-nightly-release-controller"},
		{"disabled-ci", jobregistry.ReleaseControllerRoleDisabled, false, "5.1.0-0.ci", jobregistry.ReleaseControllerStreamKindCI, "other"},
		{"end-of-life-nightly", jobregistry.ReleaseControllerRoleBlocking, true, "4.9.0-0.nightly", jobregistry.ReleaseControllerStreamKindNightly, "other"},
		{"unknown-stream", jobregistry.ReleaseControllerRoleBlocking, false, "unknown", "", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := participant(testJob(tc.name, "main", "kubevirt"), tc.role, tc.eol)
			job.ReleaseController[0].Stream.Name = tc.stream
			job.ReleaseController[0].Stream.Kind = tc.kind
			registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{job}}
			catalog, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			importance := catalog.Jobs[0].TeamMetadata.Importance
			if importance.Group != tc.group || importance.ResolvedCloud != "" || importance.ResolvedRelease != "" {
				t.Fatalf("importance = %+v", importance)
			}
		})
	}
}

func TestReleaseControllerPriorityAndMultipleParticipations(t *testing.T) {
	policy, _ := DefaultPolicy()
	ci := participant(testJob("ci", "4.15", "gcp"), jobregistry.ReleaseControllerRoleBlocking, false)
	ci.ReleaseController[0].Stream = jobregistry.ReleaseControllerStream{Name: "4.15.0-0.ci", Kind: jobregistry.ReleaseControllerStreamKindCI}
	nightly := participant(testJob("nightly", "5.1", "aws"), jobregistry.ReleaseControllerRoleInforming, false)
	nightly.ReleaseController[0].Stream = jobregistry.ReleaseControllerStream{Name: "5.1.0-0.nightly-multi", Kind: jobregistry.ReleaseControllerStreamKindNightly, Architecture: "multi"}
	hypershift := participant(testJob("hypershift", "4.14", "gcp"), jobregistry.ReleaseControllerRoleAsync, false)
	required := testJob("required", "5.1", "aws")
	required.Type = jobregistry.JobTypePresubmit
	required.Presubmit = &jobregistry.Presubmit{Required: true, AlwaysRun: true, SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
	multiple := testJob("multiple", "4.14", "gcp")
	multiple.ReleaseController = []jobregistry.ReleaseControllerParticipation{nightly.ReleaseController[0], ci.ReleaseController[0], hypershift.ReleaseController[0]}
	disabledCI := ci.ReleaseController[0]
	disabledCI.Verification.Disabled = true
	disabledHigher := testJob("disabled-higher", "5.1", "aws")
	disabledHigher.ReleaseController = []jobregistry.ReleaseControllerParticipation{disabledCI, nightly.ReleaseController[0]}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{required, nightly, ci, hypershift, multiple, disabledHigher, testJob("other", "5.1", "aws")}}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, entry := range catalog.Jobs {
		ids = append(ids, entry.JobID)
	}
	want := []string{"hypershift", "multiple", "ci", "disabled-higher", "nightly", "required", "other"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("controller order = %v, want %v", ids, want)
	}
	slices.Reverse(multiple.ReleaseController)
	shuffled, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog.Jobs, shuffled.Jobs) {
		t.Fatal("participation order changed ranking")
	}
	combined := Group{Match: "release-controller", Streams: []string{"5.1.0-0.nightly-multi"}, StreamKinds: []jobregistry.ReleaseControllerStreamKind{jobregistry.ReleaseControllerStreamKindCI}, Roles: []jobregistry.ReleaseControllerRole{jobregistry.ReleaseControllerRoleBlocking, jobregistry.ReleaseControllerRoleInforming}}
	if matches(multiple, combined) {
		t.Fatal("stream name and kind matched different participations")
	}
}

func TestOverridesAndConfigurableDimensionOrder(t *testing.T) {
	policy, _ := DefaultPolicy()
	policy.Dimensions = []string{"cloud", "release"}
	policy.Labels = []string{"team"}
	policy.Annotations = map[string]string{"owner": "default"}
	policy.Groups[0].HealthObjectives = map[jobregistry.JobType]HealthObjective{jobregistry.JobTypePeriodic: passRateObjective("1w", 9000, 20)}
	overrideObjective := passRateObjective("2w", 9500, 30)
	policy.Overrides = map[string]Override{"promoted": {Group: policy.Groups[0].ID, Rationale: "Reviewed critical coverage", Labels: []string{"critical", "team"}, Annotations: map[string]string{"owner": "alice"}, HealthObjective: &overrideObjective}}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{testJob("new-gcp", "5.1", "gcp"), testJob("old-aws", "4.9", "aws"), testJob("promoted", "", "")}}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Jobs[0].JobID != "promoted" || catalog.Jobs[1].JobID != "old-aws" {
		t.Fatalf("unexpected order: %+v", catalog.Jobs)
	}
	metadata := catalog.Jobs[0].TeamMetadata
	if metadata.HealthObjective.Checks.PassRate.MinimumPassRateBPS != 9500 || metadata.Annotations["owner"] != "alice" || !reflect.DeepEqual(metadata.Labels, []string{"critical", "team"}) || len(metadata.Importance.MatchedRules) != 2 {
		t.Fatalf("override metadata: %+v", metadata)
	}
	if policy.Annotations["owner"] != "default" || len(policy.Labels) != 1 {
		t.Fatal("policy mutated")
	}
}

func TestNonE2EPresubmitPriority(t *testing.T) {
	for _, required := range []bool{true, false} {
		name := "optional"
		if required {
			name = "required-auto-run"
		}
		t.Run(name, func(t *testing.T) {
			policy, _ := DefaultPolicy()
			presubmit := func(id, release, cloud string, framework jobregistry.E2EFramework) jobregistry.Job {
				job := testJob(id, release, cloud)
				job.Type = jobregistry.JobTypePresubmit
				job.E2EFramework = framework
				job.Presubmit = &jobregistry.Presubmit{Required: required, AlwaysRun: true, TargetRelease: "5.1", SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
				return job
			}
			periodic := participant(testJob("controller", "4.14", "gcp"), jobregistry.ReleaseControllerRoleBlocking, false)
			periodic.E2EFramework = jobregistry.E2EFrameworkV1
			registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{
				presubmit("e2e-v2", "5.1", "aws", jobregistry.E2EFrameworkV2),
				presubmit("e2e-v1", "5.1", "aws", jobregistry.E2EFrameworkV1),
				presubmit("non-e2e-old", "4.14", "gcp", jobregistry.E2EFrameworkNone),
				presubmit("non-e2e-new", "5.0", "azure", jobregistry.E2EFrameworkNone),
				presubmit("non-e2e-upcoming", "5.2", "aws", jobregistry.E2EFrameworkNone),
				presubmit("unspecified", "5.1", "aws", ""), periodic,
			}}
			catalog, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, entry := range catalog.Jobs {
				ids = append(ids, entry.JobID)
			}
			want := []string{"controller", "non-e2e-new", "non-e2e-old", "e2e-v1", "e2e-v2", "unspecified", "non-e2e-upcoming"}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("order = %v, want %v", ids, want)
			}
			slices.Reverse(registry.Jobs)
			reversed, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(catalog.Jobs, reversed.Jobs) {
				t.Fatal("input order changed framework priority")
			}
			policy.Dimensions = []string{"release", "cloud"}
			withoutPreference, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			if withoutPreference.Jobs[1].JobID != "e2e-v1" {
				t.Fatal("framework preference applied when omitted from policy")
			}
		})
	}
}

func TestPeriodicFrameworkDoesNotAffectRank(t *testing.T) {
	policy, _ := DefaultPolicy()
	older := testJob("older-non-e2e", "4.14", "aws")
	older.E2EFramework = jobregistry.E2EFrameworkNone
	newer := testJob("newer-e2e", "5.1", "gcp")
	newer.E2EFramework = jobregistry.E2EFrameworkV1
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{older, newer}}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Jobs[0].JobID != newer.ID {
		t.Fatal("non-E2E framework promoted a periodic")
	}
}

func TestPresubmitBranchPriority(t *testing.T) {
	for _, config := range []struct{ branch, release, older string }{{"main", "5.1", "5.0"}, {"trunk", "7.0", "6.9"}} {
		t.Run(config.branch, func(t *testing.T) {
			policy, _ := DefaultPolicy()
			policy.DevelopmentBranch, policy.DevelopmentRelease = config.branch, config.release
			presubmit := func(id, branch, target, tested string, framework jobregistry.E2EFramework) jobregistry.Job {
				job := testJob(id, tested, "aws")
				job.Type, job.E2EFramework = jobregistry.JobTypePresubmit, framework
				job.Presubmit = &jobregistry.Presubmit{TargetBranch: branch, TargetRelease: target, Required: true, AlwaysRun: true, SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
				return job
			}
			mainImages := presubmit("development-images", config.branch, config.release, "", jobregistry.E2EFrameworkNone)
			mainE2E := presubmit("development-e2e", config.branch, config.release, config.older, jobregistry.E2EFrameworkV1)
			olderImages := presubmit("older-images", "release-"+config.older, config.older, config.older, jobregistry.E2EFrameworkNone)
			olderE2E := presubmit("older-e2e", "release-"+config.older, config.older, config.older, jobregistry.E2EFrameworkV1)
			prospective := presubmit("prospective-images", "release-"+config.release, config.release, config.release, jobregistry.E2EFrameworkNone)
			unpromoted := testJob("prospective-counterpart", config.older, "aws")
			prospective.Presubmit.PeriodicCounterparts = []jobregistry.PeriodicCounterpart{{JobID: unpromoted.ID, TestedRelease: config.older, Source: jobregistry.PeriodicCounterpartSourceRegistryManual, Verification: jobregistry.PeriodicCounterpartVerificationHuman, Rationale: "Verified test fixture"}}
			major, minor, _ := releaseconfig.ParseRelease(config.release)
			next := fmt.Sprintf("%d.%d", major, minor+1)
			upcoming := presubmit("upcoming-images", "release-"+next, next, next, jobregistry.E2EFrameworkNone)
			controller := participant(testJob("controller", config.older, "gcp"), jobregistry.ReleaseControllerRoleBlocking, false)
			registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{prospective, olderImages, olderE2E, mainE2E, mainImages, controller, testJob("periodic", config.release, "aws"), testJob("unknown", "", ""), unpromoted, upcoming}}
			catalog, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, entry := range catalog.Jobs {
				ids = append(ids, entry.JobID)
				importance := entry.TeamMetadata.Importance
				if (entry.JobID == mainImages.ID || entry.JobID == mainE2E.ID) && (importance.PresubmitBranchScope != "development" || importance.ResolvedRelease != config.release) {
					t.Fatalf("development branch did not resolve to development release: %+v", importance)
				}
				if entry.JobID == prospective.ID && (importance.PresubmitBranchScope != "prospective" || importance.ReleaseScope != "in-horizon") {
					t.Fatalf("prospective release branch misclassified: %+v", importance)
				}
				if entry.JobID == unpromoted.ID && len(importance.RequiredPresubmitIDs) != 0 {
					t.Fatal("prospective parent promoted its periodic counterpart")
				}
			}
			want := []string{"controller", "development-images", "development-e2e", "older-images", "older-e2e", "periodic", "prospective-counterpart", "unknown", "prospective-images", "upcoming-images"}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("branch order = %v, want %v", ids, want)
			}
			slices.Reverse(registry.Jobs)
			reversed, err := Evaluate(registry, policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(catalog.Jobs, reversed.Jobs) {
				t.Fatal("input order changed branch priority")
			}
			// Once development advances, the former prospective release branch is eligible.
			policy.DevelopmentRelease = next
			advanced, err := Evaluate(&jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{prospective, unpromoted}}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if advanced.Jobs[0].JobID != prospective.ID || advanced.Jobs[0].TeamMetadata.Importance.PresubmitBranchScope != "release" {
				t.Fatal("release branch did not become eligible when development advanced")
			}
		})
	}
}

func TestMainPresubmitPrecedesProspectiveReleaseBranch(t *testing.T) {
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := DefaultPolicy()
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Importance{}
	for _, entry := range catalog.Jobs {
		byID[entry.JobID] = entry.TeamMetadata.Importance
	}
	main := byID["pull-ci-openshift-hypershift-main-images"]
	release := byID["pull-ci-openshift-hypershift-release-5.1-images"]
	if main.Rank == 0 || release.Rank == 0 || main.Rank >= release.Rank || main.ResolvedRelease != "5.1" || main.PresubmitBranchScope != "development" || release.PresubmitBranchScope != "prospective" {
		t.Fatalf("incorrect main/release ranking: main=%+v release=%+v", main, release)
	}
}

func TestRequiredPresubmitPeriodicPriority(t *testing.T) {
	policy, _ := DefaultPolicy()
	periodics := []jobregistry.Job{
		testJob("optional-periodic", "5.1", "aws"),
		testJob("unpaired-periodic", "5.1", "aws"),
		testJob("z-required-old", "4.14", "gcp"),
		testJob("a-required-old", "4.14", "gcp"),
		testJob("required-new", "4.15", "gcp"),
		testJob("required-upcoming", "5.2", "aws"),
		participant(testJob("controller", "4.14", "gcp"), jobregistry.ReleaseControllerRoleBlocking, false),
	}
	parent := func(id string, required bool, children ...jobregistry.Job) jobregistry.Job {
		job := testJob(id, "5.1", "aws")
		job.Type = jobregistry.JobTypePresubmit
		job.E2EFramework = jobregistry.E2EFrameworkV1
		// Required conditional jobs qualify, regardless of AlwaysRun or ingestion.
		job.Presubmit = &jobregistry.Presubmit{Required: required, AlwaysRun: false, TargetBranch: "main", TargetRelease: "5.1", SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
		for _, child := range children {
			job.Presubmit.PeriodicCounterparts = append(job.Presubmit.PeriodicCounterparts, jobregistry.PeriodicCounterpart{JobID: child.ID, TestedRelease: child.Versions[0], Source: jobregistry.PeriodicCounterpartSourceRegistryManual, Verification: jobregistry.PeriodicCounterpartVerificationHuman, Rationale: "Verified test fixture counterpart"})
		}
		return job
	}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: append(periodics,
		parent("z-required-parent", true, periodics[2], periodics[3], periodics[4], periodics[5]),
		parent("a-required-parent", true, periodics[2]),
		parent("optional-parent", false, periodics[0], periodics[2]),
	)}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	index := registry.Index()
	for _, entry := range catalog.Jobs {
		if index[entry.JobID].Type == jobregistry.JobTypePeriodic {
			ids = append(ids, entry.JobID)
		}
		if entry.JobID == "z-required-old" && !reflect.DeepEqual(entry.TeamMetadata.Importance.RequiredPresubmitIDs, []string{"a-required-parent", "z-required-parent"}) {
			t.Fatalf("incorrect counterpart evidence: %+v", entry)
		}
		if (entry.JobID == "optional-periodic" || entry.JobID == "unpaired-periodic" || index[entry.JobID].Type == jobregistry.JobTypePresubmit) && len(entry.TeamMetadata.Importance.RequiredPresubmitIDs) != 0 {
			t.Fatalf("unexpected counterpart promotion: %+v", entry)
		}
	}
	want := []string{"controller", "required-new", "a-required-old", "z-required-old", "optional-periodic", "unpaired-periodic", "required-upcoming"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("periodic order = %v, want %v", ids, want)
	}
	if catalog.Jobs[0].JobID != "controller" {
		t.Fatal("counterpart preference overrode controller group precedence")
	}
	slices.Reverse(registry.Jobs)
	for i := range registry.Jobs {
		if registry.Jobs[i].Presubmit != nil {
			slices.Reverse(registry.Jobs[i].Presubmit.PeriodicCounterparts)
		}
	}
	reversed, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog.Jobs, reversed.Jobs) {
		t.Fatal("input order changed counterpart priority or evidence")
	}
	policy.Dimensions = []string{"release", "cloud"}
	withoutPreference, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	index = registry.Index()
	for _, entry := range withoutPreference.Jobs {
		if index[entry.JobID].Type != jobregistry.JobTypePeriodic || entry.JobID == "controller" {
			continue
		}
		if entry.JobID != "optional-periodic" {
			t.Fatal("counterpart preference applied when omitted from policy")
		}
		break
	}
}

func TestCatalogValidation(t *testing.T) {
	policy, _ := DefaultPolicy()
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{testJob("a", "5.1", "aws"), testJob("b", "4.9", "gcp")}}
	original, _ := Evaluate(registry, policy)
	for _, tc := range []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"rank", func(c *Catalog) { c.Jobs[0].TeamMetadata.Importance.Rank = 99 }},
		{"coverage", func(c *Catalog) { c.Jobs = c.Jobs[:1] }},
		{"order", func(c *Catalog) { slices.Reverse(c.Jobs) }},
		{"algorithm", func(c *Catalog) { c.AlgorithmVersion = "unknown" }},
		{"registry", func(c *Catalog) { c.RegistryDigest = "stale" }},
		{"policy", func(c *Catalog) { c.Policy.CloudOrder = []string{"gcp", "azure", "aws"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(original)
			var changed Catalog
			json.Unmarshal(data, &changed)
			tc.mutate(&changed)
			if err := changed.Validate(registry); err == nil {
				t.Fatal("tampered artifact accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	data, _ := json.Marshal(original)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, registry); err != nil {
		t.Fatal(err)
	}
	registry.Jobs[0].Versions = []string{"5.2"}
	if _, err := LoadFile(path, registry); err == nil {
		t.Fatal("stale catalog accepted")
	}
}

func TestPolicyValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Policy)
	}{
		{"missing-fallback", func(p *Policy) { p.Groups = p.Groups[:2] }},
		{"invalid-development-release", func(p *Policy) { p.DevelopmentRelease = "main" }},
		{"duplicate-group", func(p *Policy) { p.Groups[1].ID = p.Groups[0].ID }},
		{"invalid-stream-kind", func(p *Policy) { p.Groups[1].StreamKinds = []jobregistry.ReleaseControllerStreamKind{"unknown"} }},
		{"duplicate-stream-kind", func(p *Policy) { p.Groups[1].StreamKinds = []jobregistry.ReleaseControllerStreamKind{"ci", "ci"} }},
		{"missing-stream-selector", func(p *Policy) { p.Groups[1].StreamKinds = nil }},
		{"stream-kind-on-catch-all", func(p *Policy) {
			p.Groups[len(p.Groups)-1].StreamKinds = []jobregistry.ReleaseControllerStreamKind{"ci"}
		}},
		{"unknown-dimension", func(p *Policy) { p.Dimensions = []string{"health"} }},
		{"duplicate-dimension", func(p *Policy) { p.Dimensions = []string{"cloud", "cloud"} }},
		{"invalid-alias", func(p *Policy) { p.CloudAliases["aro"] = "unknown" }},
		{"invalid-rate", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate.MinimumPassRateBPS = 10001
		}},
		{"missing-rationale", func(p *Policy) { p.Overrides = map[string]Override{"a": {Group: "other"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy, _ := DefaultPolicy()
			tc.mutate(&policy)
			if err := policy.Validate(); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	if _, err := ParsePolicy(append(append([]byte{}, defaultPolicy...), []byte("\nunknown_field: true\n")...)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ParsePolicy(append(append([]byte{}, defaultPolicy...), []byte("\n---\nteam: other\n")...)); err == nil {
		t.Fatal("extra YAML document accepted")
	}
	policy, _ := DefaultPolicy()
	policy.Overrides = map[string]Override{"missing-job": {Rationale: "Reviewed"}}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{}}
	if _, err := Evaluate(registry, policy); err == nil {
		t.Fatal("unknown override job accepted")
	}
}

func TestReleaseHorizon(t *testing.T) {
	policy, err := DefaultPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if policy.DevelopmentBranch != releaseconfig.DevelopmentBranch || policy.DevelopmentRelease != releaseconfig.DevelopmentRelease {
		t.Fatal("policy does not use shared defaults")
	}
	futureBlocker := participant(testJob("upcoming-blocker", "5.2", "aws"), jobregistry.ReleaseControllerRoleBlocking, false)
	compatibility := testJob("upcoming-target", "5.0", "aws")
	compatibility.Type = jobregistry.JobTypePresubmit
	compatibility.Presubmit = &jobregistry.Presubmit{TargetBranch: "release-5.2", TargetRelease: "5.2", Required: true, AlwaysRun: true, SippyIngestion: jobregistry.SippyIngestion{Basis: "Test"}}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{
		futureBlocker, compatibility, testJob("current", "5.1", "gcp"), testJob("older", "4.10", "aws"), testJob("unknown", "", ""),
	}}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range catalog.Jobs {
		ids = append(ids, entry.JobID)
	}
	if !reflect.DeepEqual(ids, []string{"current", "older", "unknown", "upcoming-blocker", "upcoming-target"}) {
		t.Fatalf("horizon ordering: %v", ids)
	}
	metadata := catalog.Jobs[4].TeamMetadata.Importance
	if metadata.ReleaseScope != "upcoming" || metadata.ResolvedRelease != "5.2" || metadata.ScopeRelease != "5.2" {
		t.Fatalf("target-release classification: %+v", metadata)
	}
	policy.DevelopmentRelease = "5.2"
	advanced, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Jobs[0].JobID != "upcoming-blocker" || advanced.Jobs[0].TeamMetadata.Importance.ReleaseScope != "in-horizon" {
		t.Fatal("explicit horizon did not take effect")
	}
	if err := catalog.Validate(registry); err != nil {
		t.Fatalf("historical snapshot depends on later policy defaults: %v", err)
	}
}
