package sippy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"
)

const CurrentObservationAPIVersion = "sippy-observation/v1"

type JobTier string

const (
	JobTierStandard  JobTier = "standard"
	JobTierCandidate JobTier = "candidate"
	JobTierHidden    JobTier = "hidden"
)

type ComponentReadinessMembership struct {
	Release     string  `json:"release"`
	ProwJobName string  `json:"prow_job_name"`
	Tier        JobTier `json:"tier"`
}

type AnalysisTarget struct {
	Release     string `json:"release"`
	ProwJobName string `json:"prow_job_name"`
}

type JobAnalysis struct {
	Target   AnalysisTarget            `json:"target"`
	ByPeriod map[string]AnalysisPeriod `json:"by_period"`
}

type CollectionResult struct {
	Kind        string `json:"kind"`
	Release     string `json:"release,omitempty"`
	ProwJobName string `json:"prow_job_name,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Observation struct {
	APIVersion     string                         `json:"api_version"`
	PlanDigest     string                         `json:"plan_digest"`
	SourceBaseURL  string                         `json:"source_base_url"`
	Releases       []string                       `json:"releases"`
	StartedAt      time.Time                      `json:"started_at"`
	ObservedAt     time.Time                      `json:"observed_at"`
	Complete       bool                           `json:"complete"`
	Memberships    []ComponentReadinessMembership `json:"memberships"`
	Analyses       []JobAnalysis                  `json:"analyses"`
	RecentFailures []RecentFailure                `json:"recent_failures"`
	Collection     []CollectionResult             `json:"collection"`
}

func (o *Observation) Validate(planDigest string) error {
	if o.APIVersion != CurrentObservationAPIVersion {
		return fmt.Errorf("unsupported Sippy observation API version %q", o.APIVersion)
	}
	if o.PlanDigest != planDigest {
		return fmt.Errorf("Sippy observation plan digest %q does not match plan %q", o.PlanDigest, planDigest)
	}
	if o.SourceBaseURL == "" {
		return fmt.Errorf("Sippy observation source base URL is required")
	}
	if o.ObservedAt.Before(o.StartedAt) {
		return fmt.Errorf("observation precedes collection start")
	}
	hasFailure := false
	for _, result := range o.Collection {
		if result.Error != "" {
			hasFailure = true
			break
		}
	}
	if o.Complete == hasFailure {
		return fmt.Errorf("observation completeness disagrees with collection results")
	}
	memberships := map[AnalysisTarget]JobTier{}
	for _, membership := range o.Memberships {
		if !slices.Contains(o.Releases, membership.Release) {
			return fmt.Errorf("Component Readiness membership uses release %q outside observation scope", membership.Release)
		}
		key := AnalysisTarget{Release: membership.Release, ProwJobName: membership.ProwJobName}
		if previous, found := memberships[key]; found {
			return fmt.Errorf("duplicate Component Readiness membership %q/%q (%q and %q)", key.Release, key.ProwJobName, previous, membership.Tier)
		}
		memberships[key] = membership.Tier
	}
	analyses := map[AnalysisTarget]bool{}
	for _, analysis := range o.Analyses {
		if analysis.Target.Release != "Presubmits" && !slices.Contains(o.Releases, analysis.Target.Release) {
			return fmt.Errorf("job analysis uses release %q outside observation scope", analysis.Target.Release)
		}
		if analyses[analysis.Target] {
			return fmt.Errorf("duplicate job analysis %q/%q", analysis.Target.Release, analysis.Target.ProwJobName)
		}
		analyses[analysis.Target] = true
	}
	return nil
}

func ObservationDigest(observation *Observation) (string, error) {
	data, err := json.Marshal(observation)
	if err != nil {
		return "", fmt.Errorf("encode Sippy observation digest: %w", err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func LoadObservationFile(path, planDigest string) (*Observation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Sippy observation %q: %w", path, err)
	}
	var observation Observation
	if err := json.Unmarshal(data, &observation); err != nil {
		return nil, fmt.Errorf("load Sippy observation %q: %w", path, err)
	}
	if err := observation.Validate(planDigest); err != nil {
		return nil, fmt.Errorf("validate Sippy observation %q: %w", path, err)
	}
	return &observation, nil
}
