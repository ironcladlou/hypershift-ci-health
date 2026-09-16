package jobs

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

type PeriodicJobConfig struct {
	Job         *jobregistry.Job
	Counterpart jobregistry.PeriodicCounterpart
}

type BlockingJobConfig struct {
	Job       *jobregistry.Job
	Role      string
	Periodics []PeriodicJobConfig
}

// PayloadBlockingJobConfig is a periodic job that gates a release payload.
// These relationships come directly from release-controller metadata in the
// generated registry rather than from its provisional presubmit relationships.
type PayloadBlockingJobConfig struct {
	Job            *jobregistry.Job
	Release        string
	Participations []jobregistry.ReleaseControllerParticipation
}

type JobTier string

const (
	JobTierStandard  JobTier = "standard"
	JobTierCandidate JobTier = "candidate"
	JobTierHidden    JobTier = "hidden"
)

// ComponentReadinessMembership is release-scoped policy supplied by Sippy.
// It deliberately contains no static job-definition metadata.
type ComponentReadinessMembership struct {
	Release     string
	ProwJobName string
	Tier        JobTier
}

// ComponentReadinessJobConfig joins Sippy's authoritative tier membership to
// optional static metadata from the generated job registry.
type ComponentReadinessJobConfig struct {
	Membership ComponentReadinessMembership
	Job        *jobregistry.Job
}

// AnalysisTarget identifies one registry job in one Sippy release namespace.
type AnalysisTarget struct {
	Release string
	Job     *jobregistry.Job
}

// Catalog is the validated, registry-backed view used by health reports.
type Catalog struct {
	BlockingJobs        []BlockingJobConfig
	PayloadBlockingJobs []PayloadBlockingJobConfig
	registryIndex       map[string]*jobregistry.Job
}

func NewCatalog(registry *jobregistry.Registry) (*Catalog, error) {
	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("validate job registry: %w", err)
	}
	index := registry.Index()
	catalog := &Catalog{
		BlockingJobs:  make([]BlockingJobConfig, 0),
		registryIndex: index,
	}

	for i := range registry.Jobs {
		presubmit := &registry.Jobs[i]
		if presubmit.Type != "presubmit" || presubmit.Presubmit == nil || !presubmit.Presubmit.Required || presubmit.E2EFramework == "none" || presubmit.Presubmit.TargetRelease == "" {
			continue
		}
		if slices.Contains(presubmit.Versions, "4.23") {
			continue
		}
		if presubmit.Presubmit.TargetRelease == registry.PresubmitPolicy.DevelopmentRelease && presubmit.Presubmit.TargetBranch != registry.PresubmitPolicy.DevelopmentBranch {
			continue
		}
		if releaseRank(presubmit.Presubmit.TargetRelease) > releaseRank(registry.PresubmitPolicy.DevelopmentRelease) {
			continue
		}

		configured := BlockingJobConfig{
			Job: presubmit,
		}
		for _, counterpart := range presubmit.Presubmit.PeriodicCounterparts {
			periodic := index[counterpart.JobID]
			configured.Periodics = append(configured.Periodics, PeriodicJobConfig{
				Job:         periodic,
				Counterpart: counterpart,
			})
		}
		if !supportedRelease(presubmit.Presubmit.TargetRelease) {
			continue
		}
		catalog.BlockingJobs = append(catalog.BlockingJobs, configured)
	}

	type payloadKey struct {
		release string
		jobID   string
	}
	payloadIndices := make(map[payloadKey]int)
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type != "periodic" {
			continue
		}
		for _, participation := range job.ReleaseController {
			if participation.Verification.Role != "blocking" {
				continue
			}
			if participation.Stream.EndOfLife {
				continue
			}
			if !supportedRelease(participation.Stream.Release) {
				continue
			}
			key := payloadKey{release: participation.Stream.Release, jobID: job.ID}
			index, found := payloadIndices[key]
			if !found {
				index = len(catalog.PayloadBlockingJobs)
				payloadIndices[key] = index
				catalog.PayloadBlockingJobs = append(catalog.PayloadBlockingJobs, PayloadBlockingJobConfig{
					Job:            job,
					Release:        participation.Stream.Release,
					Participations: []jobregistry.ReleaseControllerParticipation{},
				})
			}
			catalog.PayloadBlockingJobs[index].Participations = append(
				catalog.PayloadBlockingJobs[index].Participations,
				participation,
			)
		}
	}
	for i := range catalog.PayloadBlockingJobs {
		sort.Slice(catalog.PayloadBlockingJobs[i].Participations, func(a, b int) bool {
			left := catalog.PayloadBlockingJobs[i].Participations[a]
			right := catalog.PayloadBlockingJobs[i].Participations[b]
			if left.Stream.Name != right.Stream.Name {
				return left.Stream.Name < right.Stream.Name
			}
			return left.Verification.Name < right.Verification.Name
		})
	}
	sort.Slice(catalog.PayloadBlockingJobs, func(i, j int) bool {
		a, b := catalog.PayloadBlockingJobs[i], catalog.PayloadBlockingJobs[j]
		if a.Release != b.Release {
			return releaseRank(a.Release) > releaseRank(b.Release)
		}
		return a.Job.Name < b.Job.Name
	})
	releases := catalog.Releases()
	for i := range catalog.BlockingJobs {
		catalog.BlockingJobs[i].Role = roleForRelease(catalog.BlockingJobs[i].Job.Presubmit.TargetRelease, releases)
	}
	return catalog, nil
}

// ComponentReadinessJobs selects Sippy's standard tier and enriches it from
// the registry without allowing registry contents to determine membership.
func (c *Catalog) ComponentReadinessJobs(memberships []ComponentReadinessMembership) []ComponentReadinessJobConfig {
	result := make([]ComponentReadinessJobConfig, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Tier != JobTierStandard || !supportedRelease(membership.Release) {
			continue
		}
		config := ComponentReadinessJobConfig{
			Membership: membership,
			Job:        c.registryIndex[membership.ProwJobName],
		}
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Membership.Release != result[j].Membership.Release {
			return releaseRank(result[i].Membership.Release) > releaseRank(result[j].Membership.Release)
		}
		return result[i].Membership.ProwJobName < result[j].Membership.ProwJobName
	})
	return result
}

// supportedRelease bounds the health views. OpenShift 5.0 follows
// 4.22; 4.23 names the same release line and is intentionally not displayed.
func supportedRelease(release string) bool {
	if release == "4.23" {
		return false
	}
	return releaseRank(release) >= releaseRank("4.14")
}

func releaseRank(release string) int {
	majorText, minorText, found := strings.Cut(release, ".")
	if !found {
		return -1
	}
	major, majorErr := strconv.Atoi(majorText)
	minor, minorErr := strconv.Atoi(minorText)
	if majorErr != nil || minorErr != nil || minor < 0 {
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

// DisplayName removes standard Prow job prefixes used only for global identity.
func DisplayName(name string) string {
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-main-"); value != name {
		return value
	}
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-release-"); value != name {
		if _, short, found := strings.Cut(value, "-"); found {
			return short
		}
	}
	if _, value, found := strings.Cut(name, "-periodics-"); found {
		return value
	}
	return name
}

func roleForRelease(release string, releases []string) string {
	for i := len(releases) - 1; i >= 0; i-- {
		if releases[i] != release {
			continue
		}
		offset := len(releases) - 1 - i
		if offset == 0 {
			return "future"
		}
		return fmt.Sprintf("n-%d", offset)
	}
	return "unknown"
}

func RoleLabel(role, release string) string {
	if role == "future" {
		return fmt.Sprintf("Future (%s)", release)
	}
	if offset, found := strings.CutPrefix(role, "n-"); found {
		return fmt.Sprintf("N-%s (%s)", offset, release)
	}
	return fmt.Sprintf("Unknown (%s)", release)
}

func (c *Catalog) Releases() []string {
	seen := map[string]bool{}
	var releases []string
	add := func(release string) {
		if !seen[release] {
			seen[release] = true
			releases = append(releases, release)
		}
	}
	for _, job := range c.BlockingJobs {
		add(job.Job.Presubmit.TargetRelease)
	}
	for _, job := range c.PayloadBlockingJobs {
		add(job.Release)
	}
	sort.Slice(releases, func(i, j int) bool {
		return releaseRank(releases[i]) < releaseRank(releases[j])
	})
	return releases
}

func (c *Catalog) Platforms() []string {
	seen := map[string]bool{}
	var platforms []string
	add := func(values []string) {
		for _, platform := range values {
			if !seen[platform] {
				seen[platform] = true
				platforms = append(platforms, platform)
			}
		}
	}
	for _, job := range c.BlockingJobs {
		add(job.Job.Platforms)
	}
	for _, job := range c.PayloadBlockingJobs {
		add(job.Job.Platforms)
	}
	sort.Strings(platforms)
	return platforms
}

// AnalysisTargets returns the deduplicated static Sippy query plan. Component
// Readiness targets are added after their membership is fetched from Sippy.
func (c *Catalog) AnalysisTargets() []AnalysisTarget {
	type targetKey struct {
		release string
		jobID   string
	}
	seen := make(map[targetKey]struct{})
	var targets []AnalysisTarget
	add := func(release string, job *jobregistry.Job) {
		key := targetKey{release: release, jobID: job.ID}
		if _, found := seen[key]; found {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, AnalysisTarget{Release: release, Job: job})
	}
	for _, job := range c.BlockingJobs {
		if job.Job.Presubmit.SippyIngestion.Enabled {
			add("Presubmits", job.Job)
		}
	}
	for _, job := range c.BlockingJobs {
		for _, periodic := range job.Periodics {
			add(periodic.Counterpart.TestedRelease, periodic.Job)
		}
	}
	for _, job := range c.PayloadBlockingJobs {
		add(job.Release, job.Job)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Release != targets[j].Release {
			return targets[i].Release < targets[j].Release
		}
		return targets[i].Job.Name < targets[j].Job.Name
	})
	return targets
}
