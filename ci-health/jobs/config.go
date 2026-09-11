package jobs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

type Role string

const (
	RoleFuture  Role = "future"
	RoleNMinus1 Role = "n-1"
	RoleNMinus2 Role = "n-2"
)

// Mapping is report configuration for the relationship that is not available
// in the generated registry. It contains only stable registry IDs; all job
// definitions and metadata are resolved from the registry at startup.
type Mapping struct {
	PresubmitID string
	PeriodicIDs []string
	Role        Role
}

var Mappings = []Mapping{
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-upgrade"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-azure-self-managed", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-azure-self-managed"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-v2-gke", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-gke"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi"}, Role: RoleFuture},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-5-0", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn"}, Role: RoleNMinus1},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aks-5-0", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aks"}, Role: RoleNMinus1},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.22-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"}, Role: RoleNMinus2},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.22-e2e-aks", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks"}, Role: RoleNMinus2},
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
	Role        Role
	Periodics   []PeriodicJobConfig
	Job         *jobregistry.Job
}

// PayloadBlockingJobConfig is a periodic job that gates a release payload.
// These relationships come directly from release-controller metadata in the
// generated registry, rather than from the explicit presubmit mapping above.
type PayloadBlockingJobConfig struct {
	ID                 string
	Name               string
	ProwJobName        string
	Release            string
	Platforms          []string
	Stream             jobregistry.ReleaseControllerStream
	Verification       jobregistry.ReleaseControllerVerification
	MappedPresubmitIDs []string
	Job                *jobregistry.Job
}

// Catalog is the validated, registry-backed view used by health reports.
type Catalog struct {
	BlockingJobs        []BlockingJobConfig
	PayloadBlockingJobs []PayloadBlockingJobConfig
}

func NewCatalog(registry *jobregistry.Registry) (*Catalog, error) {
	index := registry.Index()
	catalog := &Catalog{BlockingJobs: make([]BlockingJobConfig, 0, len(Mappings))}
	seenPresubmits := make(map[string]struct{}, len(Mappings))

	for _, mapping := range Mappings {
		if mapping.Role != RoleFuture && mapping.Role != RoleNMinus1 && mapping.Role != RoleNMinus2 {
			return nil, fmt.Errorf("presubmit mapping %q has unsupported role %q", mapping.PresubmitID, mapping.Role)
		}
		if len(mapping.PeriodicIDs) == 0 {
			return nil, fmt.Errorf("presubmit mapping %q has no periodics", mapping.PresubmitID)
		}
		if _, found := seenPresubmits[mapping.PresubmitID]; found {
			return nil, fmt.Errorf("duplicate presubmit mapping %q", mapping.PresubmitID)
		}
		seenPresubmits[mapping.PresubmitID] = struct{}{}
		presubmit := index[mapping.PresubmitID]
		if presubmit == nil {
			return nil, fmt.Errorf("mapped presubmit %q is absent from the job registry", mapping.PresubmitID)
		}
		if presubmit.Type != "presubmit" {
			return nil, fmt.Errorf("mapped presubmit %q has registry type %q", mapping.PresubmitID, presubmit.Type)
		}
		if presubmit.Presubmit == nil || !presubmit.Presubmit.Required {
			return nil, fmt.Errorf("mapped presubmit %q is not required", mapping.PresubmitID)
		}

		configured := BlockingJobConfig{
			ID:          presubmit.ID,
			Name:        shortName(presubmit.Name),
			ProwJobName: presubmit.Name,
			Platforms:   append([]string(nil), presubmit.Platforms...),
			Role:        mapping.Role,
			Job:         presubmit,
		}
		seenPeriodics := make(map[string]struct{}, len(mapping.PeriodicIDs))
		for _, periodicID := range mapping.PeriodicIDs {
			if _, found := seenPeriodics[periodicID]; found {
				return nil, fmt.Errorf("presubmit mapping %q contains duplicate periodic %q", mapping.PresubmitID, periodicID)
			}
			seenPeriodics[periodicID] = struct{}{}
			periodic := index[periodicID]
			if periodic == nil {
				return nil, fmt.Errorf("mapped periodic %q is absent from the job registry", periodicID)
			}
			if periodic.Type != "periodic" {
				return nil, fmt.Errorf("mapped periodic %q has registry type %q", periodicID, periodic.Type)
			}
			if len(periodic.Versions) != 1 {
				return nil, fmt.Errorf("mapped periodic %q has %d versions; expected exactly one", periodicID, len(periodic.Versions))
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

	trackedReleases := make(map[string]struct{})
	mappedPresubmits := make(map[string][]string)
	for _, job := range catalog.BlockingJobs {
		for _, periodic := range job.Periodics {
			trackedReleases[periodic.Release] = struct{}{}
			mappedPresubmits[periodic.ID] = append(mappedPresubmits[periodic.ID], job.ID)
		}
	}
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
			if _, tracked := trackedReleases[participation.Stream.Release]; !tracked {
				continue
			}
			catalog.PayloadBlockingJobs = append(catalog.PayloadBlockingJobs, PayloadBlockingJobConfig{
				ID:                 job.ID,
				Name:               shortName(job.Name),
				ProwJobName:        job.Name,
				Release:            participation.Stream.Release,
				Platforms:          append([]string(nil), job.Platforms...),
				Stream:             participation.Stream,
				Verification:       participation.Verification,
				MappedPresubmitIDs: append([]string(nil), mappedPresubmits[job.ID]...),
				Job:                job,
			})
		}
	}
	sort.Slice(catalog.PayloadBlockingJobs, func(i, j int) bool {
		a, b := catalog.PayloadBlockingJobs[i], catalog.PayloadBlockingJobs[j]
		if a.Release != b.Release {
			return a.Release > b.Release
		}
		if a.Stream.Name != b.Stream.Name {
			return a.Stream.Name < b.Stream.Name
		}
		return a.ProwJobName < b.ProwJobName
	})
	return catalog, nil
}

func shortName(name string) string {
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-main-"); value != name {
		return value
	}
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-release-4.22-"); value != name {
		return value
	}
	if _, value, found := strings.Cut(name, "-periodics-"); found {
		return value
	}
	return name
}

func RoleLabel(role Role, release string) string {
	if role == RoleNMinus2 {
		return fmt.Sprintf("N-2 (%s)", release)
	}
	if role == RoleNMinus1 {
		return fmt.Sprintf("N-1 (%s)", release)
	}
	return fmt.Sprintf("Future (%s)", release)
}

func (c *Catalog) Releases() []string {
	seen := map[string]bool{}
	var releases []string
	for _, job := range c.BlockingJobs {
		for _, periodic := range job.Periodics {
			if !seen[periodic.Release] {
				seen[periodic.Release] = true
				releases = append(releases, periodic.Release)
			}
		}
	}
	return releases
}

func (c *Catalog) Platforms() []string {
	seen := map[string]bool{}
	var platforms []string
	for _, job := range c.BlockingJobs {
		for _, platform := range job.Platforms {
			if !seen[platform] {
				seen[platform] = true
				platforms = append(platforms, platform)
			}
		}
	}
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
