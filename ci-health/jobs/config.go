package jobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

// PresubmitPeriodicPairing is report configuration for the relationship that is not available
// in the generated registry. It contains only stable registry IDs; all job
// definitions and metadata are resolved from the registry at startup.
type PresubmitPeriodicPairing struct {
	PresubmitID string
	PeriodicIDs []string
}

var PresubmitPeriodicPairings = []PresubmitPeriodicPairing{
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-upgrade"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-azure-self-managed", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-azure-self-managed"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-gke", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-gke"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-5-0", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aks-5-0", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.22-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.22-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.21-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.21-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.20-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.20-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.19-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.19-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aks"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.19-e2e-conformance", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn-conformance"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.18-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.18-e2e-conformance", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn-conformance"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.17-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.17-e2e-conformance", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn-conformance"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.16-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.16-e2e-conformance", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.15-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.15-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.14-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.14-periodics-e2e-aws-ovn"}},
}

type PeriodicJobConfig struct {
	ID          string
	Name        string
	ProwJobName string
	Release     string
	Job         *jobregistry.Job
}

type BlockingJobConfig struct {
	ID          string
	Name        string
	ProwJobName string
	Platforms   []string
	Role        string
	Periodics   []PeriodicJobConfig
	Job         *jobregistry.Job
}

// PayloadBlockingJobConfig is a periodic job that gates a release payload.
// These relationships come directly from release-controller metadata in the
// generated registry, rather than from the explicit presubmit/periodic pairings above.
type ReleasePayloadParticipation struct {
	Stream       jobregistry.ReleaseControllerStream
	Verification jobregistry.ReleaseControllerVerification
}

type PayloadBlockingJobConfig struct {
	ID             string
	Name           string
	ProwJobName    string
	Release        string
	Platforms      []string
	Participations []ReleasePayloadParticipation
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
	ID              string
	Name            string
	ProwJobName     string
	Release         string
	Tier            JobTier
	Platforms       []string
	RegistryMissing bool
}

// Catalog is the validated, registry-backed view used by health reports.
type Catalog struct {
	BlockingJobs        []BlockingJobConfig
	PayloadBlockingJobs []PayloadBlockingJobConfig
	registryIndex       map[string]*jobregistry.Job
}

func NewCatalog(registry *jobregistry.Registry) (*Catalog, error) {
	index := registry.Index()
	catalog := &Catalog{
		BlockingJobs:  make([]BlockingJobConfig, 0, len(PresubmitPeriodicPairings)),
		registryIndex: index,
	}
	seenPresubmits := make(map[string]struct{}, len(PresubmitPeriodicPairings))

	for _, pairing := range PresubmitPeriodicPairings {
		if len(pairing.PeriodicIDs) == 0 {
			return nil, fmt.Errorf("presubmit pairing %q has no periodics", pairing.PresubmitID)
		}
		if _, found := seenPresubmits[pairing.PresubmitID]; found {
			return nil, fmt.Errorf("duplicate presubmit pairing %q", pairing.PresubmitID)
		}
		seenPresubmits[pairing.PresubmitID] = struct{}{}
		presubmit := index[pairing.PresubmitID]
		if presubmit == nil {
			return nil, fmt.Errorf("configured presubmit %q is absent from the job registry", pairing.PresubmitID)
		}
		if presubmit.Type != "presubmit" {
			return nil, fmt.Errorf("configured presubmit %q has registry type %q", pairing.PresubmitID, presubmit.Type)
		}
		if presubmit.Presubmit == nil || !presubmit.Presubmit.Required {
			return nil, fmt.Errorf("configured presubmit %q is not required", pairing.PresubmitID)
		}

		configured := BlockingJobConfig{
			ID:          presubmit.ID,
			Name:        shortName(presubmit.Name),
			ProwJobName: presubmit.Name,
			Platforms:   append([]string(nil), presubmit.Platforms...),
			Job:         presubmit,
		}
		seenPeriodics := make(map[string]struct{}, len(pairing.PeriodicIDs))
		for _, periodicID := range pairing.PeriodicIDs {
			if _, found := seenPeriodics[periodicID]; found {
				return nil, fmt.Errorf("presubmit pairing %q contains duplicate periodic %q", pairing.PresubmitID, periodicID)
			}
			seenPeriodics[periodicID] = struct{}{}
			periodic := index[periodicID]
			if periodic == nil {
				return nil, fmt.Errorf("paired periodic %q is absent from the job registry", periodicID)
			}
			if periodic.Type != "periodic" {
				return nil, fmt.Errorf("paired periodic %q has registry type %q", periodicID, periodic.Type)
			}
			if len(periodic.Versions) != 1 {
				return nil, fmt.Errorf("paired periodic %q has %d versions; expected exactly one", periodicID, len(periodic.Versions))
			}
			configured.Periodics = append(configured.Periodics, PeriodicJobConfig{
				ID:          periodic.ID,
				Name:        shortName(periodic.Name),
				ProwJobName: periodic.Name,
				Release:     periodic.Versions[0],
				Job:         periodic,
			})
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
					ID:             job.ID,
					Name:           shortName(job.Name),
					ProwJobName:    job.Name,
					Release:        participation.Stream.Release,
					Platforms:      append([]string(nil), job.Platforms...),
					Participations: []ReleasePayloadParticipation{},
				})
			}
			catalog.PayloadBlockingJobs[index].Participations = append(
				catalog.PayloadBlockingJobs[index].Participations,
				ReleasePayloadParticipation{Stream: participation.Stream, Verification: participation.Verification},
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
		return a.ProwJobName < b.ProwJobName
	})
	releases := catalog.Releases()
	for i := range catalog.BlockingJobs {
		catalog.BlockingJobs[i].Role = roleForRelease(catalog.BlockingJobs[i].Periodics[0].Release, releases)
	}
	return catalog, nil
}

// ComponentReadinessJobs selects Sippy's standard tier and enriches it from
// the registry without allowing registry contents to determine membership.
func (c *Catalog) ComponentReadinessJobs(memberships []ComponentReadinessMembership) []ComponentReadinessJobConfig {
	result := make([]ComponentReadinessJobConfig, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Tier != JobTierStandard {
			continue
		}
		config := ComponentReadinessJobConfig{
			ID:          membership.ProwJobName,
			Name:        shortName(membership.ProwJobName),
			ProwJobName: membership.ProwJobName,
			Release:     membership.Release,
			Tier:        membership.Tier,
		}
		if definition := c.registryIndex[membership.ProwJobName]; definition != nil {
			config.Platforms = append([]string(nil), definition.Platforms...)
		} else {
			config.RegistryMissing = true
		}
		result = append(result, config)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Release != result[j].Release {
			return releaseRank(result[i].Release) > releaseRank(result[j].Release)
		}
		return result[i].ProwJobName < result[j].ProwJobName
	})
	return result
}

// supportedRelease bounds the payload view. OpenShift 5.0 follows
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

func shortName(name string) string {
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
		for _, periodic := range job.Periodics {
			add(periodic.Release)
		}
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
		add(job.Platforms)
	}
	for _, job := range c.PayloadBlockingJobs {
		add(job.Platforms)
	}
	sort.Strings(platforms)
	return platforms
}

func (c *Catalog) PresubmitProwJobNames() []string {
	names := make([]string, len(c.BlockingJobs))
	for i, job := range c.BlockingJobs {
		names[i] = job.ProwJobName
	}
	return names
}

func (c *Catalog) PeriodicProwJobNamesByRelease() map[string][]string {
	result := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	add := func(release, name string) {
		if seen[release] == nil {
			seen[release] = make(map[string]struct{})
		}
		if _, found := seen[release][name]; found {
			return
		}
		seen[release][name] = struct{}{}
		result[release] = append(result[release], name)
	}
	for _, job := range c.BlockingJobs {
		for _, periodic := range job.Periodics {
			add(periodic.Release, periodic.ProwJobName)
		}
	}
	for _, job := range c.PayloadBlockingJobs {
		add(job.Release, job.ProwJobName)
	}
	for release := range result {
		sort.Strings(result[release])
	}
	return result
}

func (c *Catalog) PeriodicJobCount() int {
	seen := make(map[string]struct{})
	for _, job := range c.BlockingJobs {
		for _, periodic := range job.Periodics {
			seen[periodic.ID] = struct{}{}
		}
	}
	for _, job := range c.PayloadBlockingJobs {
		seen[job.ID] = struct{}{}
	}
	return len(seen)
}
