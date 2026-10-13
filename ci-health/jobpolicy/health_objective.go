package jobpolicy

import (
	"fmt"
	"slices"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

const (
	AssessmentPresubmitSystemicHealth = "presubmit-systemic-health/v1"
	AssessmentJobPassRate             = "job-pass-rate/v1"
)

// HealthObjective is an assessment contract for external evaluators, not a
// measured result. Evaluators return satisfied, violated, or indeterminate.
// Missing evidence must never be interpreted as a satisfied objective.
type HealthObjective struct {
	Assessment           string              `json:"assessment" yaml:"assessment" enum:"presubmit-systemic-health/v1,job-pass-rate/v1" doc:"Assessment contract for an external evaluator; this catalog performs no checks"`
	Objective            string              `json:"objective" yaml:"objective" enum:"no-systemic-failures,pass-rate-at-least-threshold" doc:"Systemic health requires both checks to have sufficient evidence and no attributed systemic failure; either attributed signal violates it"`
	Checks               HealthChecks        `json:"checks" yaml:"checks"`
	Attribution          *FailureAttribution `json:"attribution,omitempty" yaml:"attribution"`
	InsufficientEvidence string              `json:"insufficient_evidence" yaml:"insufficient_evidence" enum:"indeterminate" doc:"Evaluators report satisfied, violated, or indeterminate; missing samples, identities, or unresolved attribution must not be treated as satisfied"`
}

type HealthChecks struct {
	JobPermafailing    *JobPermafailingCheck    `json:"job_permafailing,omitempty" yaml:"job_permafailing"`
	CorrelatedFailures *CorrelatedFailuresCheck `json:"correlated_failures,omitempty" yaml:"correlated_failures"`
	PassRate           *PassRateCheck           `json:"pass_rate,omitempty" yaml:"pass_rate"`
}

// JobPermafailingCheck counts distinct PR identities, not reruns or commits.
// A PR is successful if any completed run of this job passed within the window.
// Reaching the sample minimum while meeting the successful-PR ceiling signals
// suspected permafailing; attribution is still required before a violation.
type JobPermafailingCheck struct {
	Window               string `json:"window" yaml:"window" enum:"4h,24h,48h,1w,2w,1m"`
	MinimumDistinctPRs   int    `json:"minimum_distinct_prs" yaml:"minimum_distinct_prs" minimum:"2" doc:"Minimum PR identities (repository and PR number); reruns and commits on one PR count once"`
	MaximumSuccessfulPRs int    `json:"maximum_successful_prs" yaml:"maximum_successful_prs" minimum:"0" doc:"At most this many PRs may have any passing completed run of this job in the window to signal suspected permafailing"`
}

// CorrelatedFailuresCheck signals a shared failure signature across the
// configured minimum distinct jobs and PRs in its cohort. The signature is a
// test identity or normalized infrastructure error; a generic failed-job state
// is insufficient. Retries on one PR do not establish PR independence.
type CorrelatedFailuresCheck struct {
	Window              string `json:"window" yaml:"window" enum:"4h,24h,48h,1w,2w,1m"`
	MinimumDistinctPRs  int    `json:"minimum_distinct_prs" yaml:"minimum_distinct_prs" minimum:"2" doc:"Minimum PR identities (repository and PR number); reruns and commits on one PR count once"`
	MinimumDistinctJobs int    `json:"minimum_distinct_jobs" yaml:"minimum_distinct_jobs" minimum:"2"`
	Cohort              string `json:"cohort" yaml:"cohort" doc:"Reference to the policy's named cohort selector"`
	Signature           string `json:"signature" yaml:"signature" enum:"test-or-infrastructure-error" doc:"Same failing test identity or normalized infrastructure error across the required distinct jobs and PRs; generic failed-job states are insufficient"`
}

// FailureAttribution requires evidence supporting a cause unrelated to the PR
// changes (such as infrastructure, job configuration, or a shared base
// regression). Failure counts alone are insufficient. A positive signal with
// unresolved attribution is indeterminate, not violated. Systemic health is
// satisfied only when both checks have sufficient evidence and no attributed
// systemic failure; either attributed signal violates the objective.
type FailureAttribution struct {
	Require string `json:"require" yaml:"require" enum:"unrelated-to-pr-contents" doc:"Require evidence of a cause unrelated to PR changes; failure counts alone are insufficient and unresolved attribution is indeterminate"`
}

// PassRateCheck uses passing completed runs divided by all completed,
// non-aborted runs (including infrastructure errors). Pending/running and
// aborted runs do not count. Below MinimumRuns the result is indeterminate;
// otherwise the observed rate must be at least MinimumPassRateBPS.
type PassRateCheck struct {
	Window             string `json:"window" yaml:"window" enum:"4h,24h,48h,1w,2w,1m"`
	MinimumPassRateBPS int    `json:"minimum_pass_rate_bps" yaml:"minimum_pass_rate_bps" minimum:"0" maximum:"10000"`
	MinimumRuns        int    `json:"minimum_runs" yaml:"minimum_runs" minimum:"1"`
	Denominator        string `json:"denominator" yaml:"denominator" enum:"completed-non-aborted-runs" doc:"All completed non-aborted runs, including infrastructure errors; pending/running and aborted runs are excluded. Numerator is passing completed runs"`
}

// HealthCohort selects comparison jobs from this team's catalog. same-as-job
// resolves against the evaluated job's exact target branch; development
// resolves against Policy.DevelopmentBranch. Missing branch identity leaves
// the assessment indeterminate. Evaluators apply all selectors together.
type HealthCohort struct {
	Scope                        string              `json:"scope" yaml:"scope" enum:"team" doc:"Resolve members from the team catalog containing this policy"`
	JobType                      jobregistry.JobType `json:"job_type" yaml:"job_type" enum:"presubmit,periodic"`
	TargetBranch                 string              `json:"target_branch,omitempty" yaml:"target_branch" enum:"same-as-job,development" doc:"same-as-job matches the assessed job exact target branch; development resolves to the policy development branch; missing identity yields indeterminate"`
	ReleaseScopes                []string            `json:"release_scopes" yaml:"release_scopes"`
	ExcludePresubmitBranchScopes []string            `json:"exclude_presubmit_branch_scopes,omitempty" yaml:"exclude_presubmit_branch_scopes"`
}

func validObjectiveWindow(window string) bool {
	return slices.Contains([]string{"4h", "24h", "48h", "1w", "2w", "1m"}, window)
}

func (cohort HealthCohort) validate() error {
	if cohort.Scope != "team" || (cohort.JobType != jobregistry.JobTypePresubmit && cohort.JobType != jobregistry.JobTypePeriodic) || len(cohort.ReleaseScopes) == 0 || !unique(cohort.ReleaseScopes) {
		return fmt.Errorf("invalid health cohort selector")
	}
	for _, scope := range cohort.ReleaseScopes {
		if !slices.Contains([]string{"in-horizon", "upcoming", "unknown"}, scope) {
			return fmt.Errorf("invalid cohort release scope %q", scope)
		}
	}
	if cohort.JobType == jobregistry.JobTypePresubmit {
		if cohort.TargetBranch != "same-as-job" && cohort.TargetBranch != "development" {
			return fmt.Errorf("presubmit cohort requires a target branch selector")
		}
	} else if cohort.TargetBranch != "" || len(cohort.ExcludePresubmitBranchScopes) > 0 {
		return fmt.Errorf("presubmit branch selectors cannot apply to periodic cohorts")
	}
	if !unique(cohort.ExcludePresubmitBranchScopes) {
		return fmt.Errorf("duplicate excluded cohort branch scope")
	}
	for _, scope := range cohort.ExcludePresubmitBranchScopes {
		if !slices.Contains([]string{"development", "release", "prospective", "unknown"}, scope) {
			return fmt.Errorf("invalid excluded cohort branch scope %q", scope)
		}
	}
	return nil
}

func (objective HealthObjective) validate(cohorts map[string]HealthCohort) error {
	if objective.InsufficientEvidence != "indeterminate" {
		return fmt.Errorf("health objective requires indeterminate on insufficient evidence")
	}
	switch objective.Assessment {
	case AssessmentJobPassRate:
		check := objective.Checks.PassRate
		if objective.Objective != "pass-rate-at-least-threshold" || check == nil || objective.Checks.JobPermafailing != nil || objective.Checks.CorrelatedFailures != nil || objective.Attribution != nil {
			return fmt.Errorf("pass-rate objective requires only a pass_rate check")
		}
		if !validObjectiveWindow(check.Window) || check.MinimumPassRateBPS < 0 || check.MinimumPassRateBPS > 10000 || check.MinimumRuns < 1 || check.Denominator != "completed-non-aborted-runs" {
			return fmt.Errorf("invalid pass-rate check")
		}
	case AssessmentPresubmitSystemicHealth:
		permafailing, correlated := objective.Checks.JobPermafailing, objective.Checks.CorrelatedFailures
		if objective.Objective != "no-systemic-failures" || permafailing == nil || correlated == nil || objective.Checks.PassRate != nil || objective.Attribution == nil || objective.Attribution.Require != "unrelated-to-pr-contents" {
			return fmt.Errorf("systemic-health objective requires both systemic checks and PR-independent attribution")
		}
		if !validObjectiveWindow(permafailing.Window) || permafailing.MinimumDistinctPRs < 2 || permafailing.MaximumSuccessfulPRs < 0 || permafailing.MaximumSuccessfulPRs >= permafailing.MinimumDistinctPRs {
			return fmt.Errorf("invalid job-permafailing check")
		}
		if !validObjectiveWindow(correlated.Window) || correlated.MinimumDistinctPRs < 2 || correlated.MinimumDistinctJobs < 2 || correlated.Signature != "test-or-infrastructure-error" {
			return fmt.Errorf("invalid correlated-failures check")
		}
		cohort, found := cohorts[correlated.Cohort]
		if !found || cohort.JobType != jobregistry.JobTypePresubmit {
			return fmt.Errorf("correlated failures require a named presubmit cohort %q", correlated.Cohort)
		}
	default:
		return fmt.Errorf("unsupported health assessment %q", objective.Assessment)
	}
	return nil
}

func validateObjectives(objectives map[jobregistry.JobType]HealthObjective, cohorts map[string]HealthCohort) error {
	for kind, objective := range objectives {
		if kind != jobregistry.JobTypePresubmit && kind != jobregistry.JobTypePeriodic {
			return fmt.Errorf("health objective uses unsupported job type %q", kind)
		}
		if err := objective.validate(cohorts); err != nil {
			return fmt.Errorf("health objective for %s: %w", kind, err)
		}
		if kind != jobregistry.JobTypePresubmit && objective.Assessment == AssessmentPresubmitSystemicHealth {
			return fmt.Errorf("presubmit systemic-health assessment cannot apply to %s jobs", kind)
		}
	}
	return nil
}
