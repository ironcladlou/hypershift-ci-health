// Package reportplan derives the deterministic, dashboard-specific selection
// of registry jobs. It contains no live service data and no presentation text.
package reportplan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

const CurrentAPIVersion = "report-plan/v1"

type SelectionPolicy struct {
	DevelopmentBranch  string   `json:"development_branch"`
	DevelopmentRelease string   `json:"development_release"`
	MinimumRelease     string   `json:"minimum_release"`
	MaximumRelease     string   `json:"maximum_release"`
	ExcludedReleases   []string `json:"excluded_releases"`
}

func DefaultSelectionPolicy(branch, release string) SelectionPolicy {
	return SelectionPolicy{DevelopmentBranch: branch, DevelopmentRelease: release, MinimumRelease: "4.14", MaximumRelease: release, ExcludedReleases: []string{"4.23"}}
}

type PeriodicReference struct {
	JobID         string                                      `json:"job_id"`
	TestedRelease string                                      `json:"tested_release"`
	Source        jobregistry.PeriodicCounterpartSource       `json:"source"`
	Verification  jobregistry.PeriodicCounterpartVerification `json:"verification"`
	Rationale     string                                      `json:"rationale"`
}

type Presubmit struct {
	JobID     string              `json:"job_id"`
	Role      string              `json:"role"`
	Periodics []PeriodicReference `json:"periodics"`
}

type PayloadJob struct {
	JobID          string                                       `json:"job_id"`
	Release        string                                       `json:"release"`
	Participations []jobregistry.ReleaseControllerParticipation `json:"participations"`
}

type AnalysisTarget struct {
	Release string `json:"release"`
	JobID   string `json:"job_id"`
}

type Plan struct {
	APIVersion      string           `json:"api_version"`
	RegistryDigest  string           `json:"registry_digest"`
	Policy          SelectionPolicy  `json:"policy"`
	Releases        []string         `json:"releases"`
	Platforms       []string         `json:"platforms"`
	Presubmits      []Presubmit      `json:"presubmits"`
	PayloadJobs     []PayloadJob     `json:"payload_jobs"`
	AnalysisTargets []AnalysisTarget `json:"analysis_targets"`
}

func RegistryDigest(registry *jobregistry.Registry) (string, error) {
	data, err := json.Marshal(registry)
	if err != nil {
		return "", fmt.Errorf("encode registry digest: %w", err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func Digest(plan *Plan) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("encode report plan digest: %w", err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func Build(registry *jobregistry.Registry, policy SelectionPolicy) (*Plan, error) {
	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("validate job registry: %w", err)
	}
	if policy.DevelopmentBranch == "" || policy.DevelopmentRelease == "" || policy.MinimumRelease == "" || policy.MaximumRelease == "" {
		return nil, fmt.Errorf("development branch, development release, minimum release, and maximum release are required")
	}
	if releaseRank(policy.DevelopmentRelease) < 0 || releaseRank(policy.MinimumRelease) < 0 || releaseRank(policy.MaximumRelease) < releaseRank(policy.MinimumRelease) {
		return nil, fmt.Errorf("development, minimum, and maximum releases must form a valid major.minor range")
	}
	digest, err := RegistryDigest(registry)
	if err != nil {
		return nil, err
	}
	index := registry.Index()
	plan := &Plan{APIVersion: CurrentAPIVersion, RegistryDigest: digest, Policy: policy, Presubmits: []Presubmit{}, PayloadJobs: []PayloadJob{}, AnalysisTargets: []AnalysisTarget{}}

	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type != jobregistry.JobTypePresubmit || job.Presubmit == nil || !job.Presubmit.Required || job.E2EFramework == jobregistry.E2EFrameworkNone || job.Presubmit.TargetRelease == "" {
			continue
		}
		if containsAny(job.Versions, policy.ExcludedReleases) || !supportedRelease(job.Presubmit.TargetRelease, policy) {
			continue
		}
		if job.Presubmit.TargetRelease == policy.DevelopmentRelease && job.Presubmit.TargetBranch != policy.DevelopmentBranch {
			continue
		}
		if releaseRank(job.Presubmit.TargetRelease) > releaseRank(policy.DevelopmentRelease) {
			continue
		}
		entry := Presubmit{JobID: job.ID, Periodics: []PeriodicReference{}}
		for _, counterpart := range job.Presubmit.PeriodicCounterparts {
			entry.Periodics = append(entry.Periodics, PeriodicReference{JobID: counterpart.JobID, TestedRelease: counterpart.TestedRelease, Source: counterpart.Source, Verification: counterpart.Verification, Rationale: counterpart.Rationale})
		}
		plan.Presubmits = append(plan.Presubmits, entry)
	}

	type payloadKey struct{ release, jobID string }
	payloadIndices := map[payloadKey]int{}
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type != jobregistry.JobTypePeriodic {
			continue
		}
		for _, participation := range job.ReleaseController {
			if participation.Verification.Role != jobregistry.ReleaseControllerRoleBlocking || participation.Stream.EndOfLife || !supportedRelease(participation.Stream.Release, policy) {
				continue
			}
			key := payloadKey{participation.Stream.Release, job.ID}
			at, found := payloadIndices[key]
			if !found {
				at = len(plan.PayloadJobs)
				payloadIndices[key] = at
				plan.PayloadJobs = append(plan.PayloadJobs, PayloadJob{JobID: job.ID, Release: participation.Stream.Release, Participations: []jobregistry.ReleaseControllerParticipation{}})
			}
			plan.PayloadJobs[at].Participations = append(plan.PayloadJobs[at].Participations, participation)
		}
	}
	for i := range plan.PayloadJobs {
		sort.Slice(plan.PayloadJobs[i].Participations, func(a, b int) bool {
			left, right := plan.PayloadJobs[i].Participations[a], plan.PayloadJobs[i].Participations[b]
			if left.Stream.Name != right.Stream.Name {
				return left.Stream.Name < right.Stream.Name
			}
			return left.Verification.Name < right.Verification.Name
		})
	}
	sort.Slice(plan.PayloadJobs, func(i, j int) bool {
		if plan.PayloadJobs[i].Release != plan.PayloadJobs[j].Release {
			return releaseRank(plan.PayloadJobs[i].Release) > releaseRank(plan.PayloadJobs[j].Release)
		}
		return index[plan.PayloadJobs[i].JobID].Name < index[plan.PayloadJobs[j].JobID].Name
	})
	plan.Releases = releases(plan, index)
	for i := range plan.Presubmits {
		plan.Presubmits[i].Role = roleForRelease(index[plan.Presubmits[i].JobID].Presubmit.TargetRelease, plan.Releases)
	}
	plan.Platforms = platforms(plan, index)
	plan.AnalysisTargets = analysisTargets(plan, index)
	if err := plan.Validate(registry); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *Plan) Validate(registry *jobregistry.Registry) error {
	if p.APIVersion != CurrentAPIVersion {
		return fmt.Errorf("unsupported report plan API version %q", p.APIVersion)
	}
	digest, err := RegistryDigest(registry)
	if err != nil {
		return err
	}
	if p.RegistryDigest != digest {
		return fmt.Errorf("report plan registry digest %q does not match registry %q", p.RegistryDigest, digest)
	}
	index := registry.Index()
	for _, entry := range p.Presubmits {
		if index[entry.JobID] == nil {
			return fmt.Errorf("report plan references unknown presubmit %q", entry.JobID)
		}
		for _, periodic := range entry.Periodics {
			if index[periodic.JobID] == nil {
				return fmt.Errorf("report plan references unknown periodic %q", periodic.JobID)
			}
		}
	}
	for _, entry := range p.PayloadJobs {
		if index[entry.JobID] == nil {
			return fmt.Errorf("report plan references unknown payload job %q", entry.JobID)
		}
	}
	for _, target := range p.AnalysisTargets {
		if index[target.JobID] == nil {
			return fmt.Errorf("report plan references unknown analysis target %q", target.JobID)
		}
	}
	return nil
}

func LoadFile(path string, registry *jobregistry.Registry) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read report plan %q: %w", path, err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("load report plan %q: %w", path, err)
	}
	if err := plan.Validate(registry); err != nil {
		return nil, fmt.Errorf("validate report plan %q: %w", path, err)
	}
	expected, err := Build(registry, plan.Policy)
	if err != nil {
		return nil, fmt.Errorf("rebuild report plan %q: %w", path, err)
	}
	if !reflect.DeepEqual(&plan, expected) {
		return nil, fmt.Errorf("validate report plan %q: artifact is not the canonical result of its registry and policy", path)
	}
	return &plan, nil
}

func supportedRelease(release string, policy SelectionPolicy) bool {
	rank := releaseRank(release)
	return !slices.Contains(policy.ExcludedReleases, release) && rank >= releaseRank(policy.MinimumRelease) && rank <= releaseRank(policy.MaximumRelease)
}
func containsAny(values, candidates []string) bool {
	for _, candidate := range candidates {
		if slices.Contains(values, candidate) {
			return true
		}
	}
	return false
}
func releaseRank(release string) int {
	majorText, minorText, ok := strings.Cut(release, ".")
	if !ok {
		return -1
	}
	major, e1 := strconv.Atoi(majorText)
	minor, e2 := strconv.Atoi(minorText)
	if e1 != nil || e2 != nil || minor < 0 {
		return -1
	}
	if major == 4 {
		return minor
	}
	if major >= 5 {
		return 23 + (major-5)*100 + minor
	}
	return -1
}
func roleForRelease(release string, releases []string) string {
	for i := len(releases) - 1; i >= 0; i-- {
		if releases[i] == release {
			offset := len(releases) - 1 - i
			if offset == 0 {
				return "future"
			}
			return fmt.Sprintf("n-%d", offset)
		}
	}
	return "unknown"
}
func releases(plan *Plan, index map[string]*jobregistry.Job) []string {
	seen := map[string]bool{}
	var result []string
	add := func(v string) {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	for _, e := range plan.Presubmits {
		add(index[e.JobID].Presubmit.TargetRelease)
	}
	for _, e := range plan.PayloadJobs {
		add(e.Release)
	}
	sort.Slice(result, func(i, j int) bool { return releaseRank(result[i]) < releaseRank(result[j]) })
	return result
}
func platforms(plan *Plan, index map[string]*jobregistry.Job) []string {
	seen := map[string]bool{}
	for _, e := range plan.Presubmits {
		for _, v := range index[e.JobID].Platforms {
			seen[v] = true
		}
	}
	for _, e := range plan.PayloadJobs {
		for _, v := range index[e.JobID].Platforms {
			seen[v] = true
		}
	}
	result := make([]string, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}
func analysisTargets(plan *Plan, index map[string]*jobregistry.Job) []AnalysisTarget {
	type key struct{ release, id string }
	seen := map[key]bool{}
	var result []AnalysisTarget
	add := func(release, id string) {
		k := key{release, id}
		if !seen[k] {
			seen[k] = true
			result = append(result, AnalysisTarget{Release: release, JobID: id})
		}
	}
	for _, e := range plan.Presubmits {
		if index[e.JobID].Presubmit.SippyIngestion.Enabled {
			add("Presubmits", e.JobID)
		}
		for _, p := range e.Periodics {
			add(p.TestedRelease, p.JobID)
		}
	}
	for _, e := range plan.PayloadJobs {
		add(e.Release, e.JobID)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Release != result[j].Release {
			return result[i].Release < result[j].Release
		}
		return index[result[i].JobID].Name < index[result[j].JobID].Name
	})
	return result
}
