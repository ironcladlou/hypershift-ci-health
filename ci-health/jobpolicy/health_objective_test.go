package jobpolicy

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

func TestHealthObjectivesByJobTypeAndOverrides(t *testing.T) {
	policy, _ := DefaultPolicy()
	periodic := testJob("periodic", "5.1", "aws")
	presubmit := testJob("presubmit", "5.1", "aws")
	presubmit.Type = jobregistry.JobTypePresubmit
	presubmit.Presubmit = &jobregistry.Presubmit{TargetBranch: "main", TargetRelease: "5.1", SippyIngestion: jobregistry.SippyIngestion{Basis: "Test fixture"}}
	registry := &jobregistry.Registry{APIVersion: jobregistry.CurrentAPIVersion, Jobs: []jobregistry.Job{periodic, presubmit}}
	catalog, err := Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Jobs {
		kind := registry.Index()[entry.JobID].Type
		if !reflect.DeepEqual(entry.TeamMetadata.HealthObjective, policy.HealthObjectives[kind]) {
			t.Fatalf("wrong default objective for %s", kind)
		}
	}
	// A periodic group replacement must leave the presubmit objective intact.
	policy.Groups[len(policy.Groups)-1].HealthObjectives = map[jobregistry.JobType]HealthObjective{jobregistry.JobTypePeriodic: passRateObjective("48h", 9000, 20)}
	data, _ := json.Marshal(policy.HealthObjectives[jobregistry.JobTypePresubmit])
	var override HealthObjective
	json.Unmarshal(data, &override)
	override.Checks.JobPermafailing.MinimumDistinctPRs = 7
	policy.Overrides = map[string]Override{"presubmit": {Rationale: "Reviewed evidence threshold", HealthObjective: &override}}
	catalog, err = Evaluate(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Jobs {
		objective := entry.TeamMetadata.HealthObjective
		if entry.JobID == "periodic" && (objective.Checks.PassRate.MinimumPassRateBPS != 9000 || objective.Checks.PassRate.Window != "48h") {
			t.Fatal("group objective not applied")
		}
		if entry.JobID == "presubmit" && objective.Checks.JobPermafailing.MinimumDistinctPRs != 7 {
			t.Fatal("job objective not applied")
		}
	}
	policy.Overrides = map[string]Override{"periodic": {Rationale: "Invalid assessment type", HealthObjective: &override}}
	if _, err := Evaluate(registry, policy); err == nil {
		t.Fatal("presubmit systemic assessment applied to a periodic")
	}
}

func TestHealthObjectiveValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Policy)
	}{
		{"missing presubmit default", func(p *Policy) { delete(p.HealthObjectives, jobregistry.JobTypePresubmit) }},
		{"missing periodic default", func(p *Policy) { delete(p.HealthObjectives, jobregistry.JobTypePeriodic) }},
		{"unknown job type", func(p *Policy) { p.HealthObjectives["postsubmit"] = p.HealthObjectives[jobregistry.JobTypePeriodic] }},
		{"unknown assessment", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePeriodic]
			o.Assessment = "unknown/v1"
			p.HealthObjectives[jobregistry.JobTypePeriodic] = o
		}},
		{"wrong objective", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePeriodic]
			o.Objective = "no-systemic-failures"
			p.HealthObjectives[jobregistry.JobTypePeriodic] = o
		}},
		{"missing pass-rate check", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePeriodic]
			o.Checks.PassRate = nil
			p.HealthObjectives[jobregistry.JobTypePeriodic] = o
		}},
		{"mixed checks", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePresubmit]
			o.Checks.PassRate = p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate
			p.HealthObjectives[jobregistry.JobTypePresubmit] = o
		}},
		{"missing systemic check", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePresubmit]
			o.Checks.CorrelatedFailures = nil
			p.HealthObjectives[jobregistry.JobTypePresubmit] = o
		}},
		{"missing attribution", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePresubmit]
			o.Attribution = nil
			p.HealthObjectives[jobregistry.JobTypePresubmit] = o
		}},
		{"wrong attribution", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePresubmit].Attribution.Require = "failure-counts-only"
		}},
		{"unsafe missing-evidence result", func(p *Policy) {
			o := p.HealthObjectives[jobregistry.JobTypePresubmit]
			o.InsufficientEvidence = "satisfied"
			p.HealthObjectives[jobregistry.JobTypePresubmit] = o
		}},
		{"insufficient independent PRs", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePresubmit].Checks.JobPermafailing.MinimumDistinctPRs = 1
		}},
		{"invalid successful PR ceiling", func(p *Policy) {
			c := p.HealthObjectives[jobregistry.JobTypePresubmit].Checks.JobPermafailing
			c.MaximumSuccessfulPRs = c.MinimumDistinctPRs
		}},
		{"invalid window", func(p *Policy) { p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate.Window = "forever" }},
		{"invalid rate", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate.MinimumPassRateBPS = 10001
		}},
		{"invalid minimum runs", func(p *Policy) { p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate.MinimumRuns = 0 }},
		{"unknown denominator", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePeriodic].Checks.PassRate.Denominator = "all-runs"
		}},
		{"insufficient correlated jobs", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePresubmit].Checks.CorrelatedFailures.MinimumDistinctJobs = 1
		}},
		{"generic failure signature", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePresubmit].Checks.CorrelatedFailures.Signature = "failure"
		}},
		{"missing cohort", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePresubmit].Checks.CorrelatedFailures.Cohort = "missing"
		}},
		{"invalid cohort branch", func(p *Policy) {
			c := p.Cohorts["team-presubmits-same-branch"]
			c.TargetBranch = "anything"
			p.Cohorts["team-presubmits-same-branch"] = c
		}},
		{"invalid cohort scope", func(p *Policy) {
			c := p.Cohorts["team-presubmits-same-branch"]
			c.ReleaseScopes = []string{"missing"}
			p.Cohorts["team-presubmits-same-branch"] = c
		}},
		{"systemic objective on periodic", func(p *Policy) {
			p.HealthObjectives[jobregistry.JobTypePeriodic] = p.HealthObjectives[jobregistry.JobTypePresubmit]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, _ := DefaultPolicy()
			test.mutate(&policy)
			if err := policy.Validate(); err == nil {
				t.Fatal("invalid objective accepted")
			}
		})
	}
	legacy := append(append([]byte{}, defaultPolicy...), []byte("\nhealth_target:\n  window: 1w\n  minimum_pass_rate_bps: 8000\n  minimum_runs: 10\n")...)
	if _, err := ParsePolicy(legacy); err == nil || !strings.Contains(err.Error(), "health_target") {
		t.Fatalf("legacy target field accepted: %v", err)
	}
}
