package jobregistry

import (
	"fmt"
	"sort"
	"strings"
)

// presubmitPeriodicOverrides contains relationships that cannot be derived
// from an exact generated-job name match. They remain provisional because the
// relationship is not expressed by Prow itself.
var presubmitPeriodicOverrides = []PresubmitPeriodicRelationship{
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-upgrade"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi"}},
	{PresubmitID: "pull-ci-openshift-hypershift-main-e2e-aws-5-0", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.22-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.21-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.20-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aws-ovn"}},
	{PresubmitID: "pull-ci-openshift-hypershift-release-4.19-e2e-aws", PeriodicIDs: []string{"periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn"}},
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

func populatePresubmitPeriodicRelationships(registry *Registry) error {
	return populatePresubmitPeriodicRelationshipsWithOverrides(registry, presubmitPeriodicOverrides)
}

func populatePresubmitPeriodicRelationshipsWithOverrides(registry *Registry, overrides []PresubmitPeriodicRelationship) error {
	index := registry.Index()
	relationships := make([]PresubmitPeriodicRelationship, 0, len(overrides))
	overridden := make(map[string]struct{}, len(overrides))

	for _, override := range overrides {
		override.Provisional = true
		override.Basis = PresubmitPeriodicRelationshipBasisManualOverride
		override.PeriodicIDs = append([]string(nil), override.PeriodicIDs...)
		relationships = append(relationships, override)
		overridden[override.PresubmitID] = struct{}{}
	}

	periodics := make(map[string][]string)
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type != "periodic" || len(job.Versions) != 1 {
			continue
		}
		if key, ok := exactReleasePeriodicKey(job.Name, job.Versions[0]); ok {
			periodics[key] = append(periodics[key], job.ID)
		}
	}

	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type != "presubmit" || len(job.Versions) != 1 {
			continue
		}
		if _, found := overridden[job.ID]; found {
			continue
		}
		key, ok := exactReleasePresubmitKey(job.Name, job.Versions[0])
		if !ok || len(periodics[key]) != 1 {
			continue
		}
		relationships = append(relationships, PresubmitPeriodicRelationship{
			PresubmitID: job.ID,
			PeriodicIDs: []string{periodics[key][0]},
			Provisional: true,
			Basis:       PresubmitPeriodicRelationshipBasisExactReleaseName,
		})
	}

	sort.Slice(relationships, func(i, j int) bool {
		return relationships[i].PresubmitID < relationships[j].PresubmitID
	})
	registry.PresubmitPeriodicRelationships = relationships
	if err := validatePresubmitPeriodicRelationships(registry, index); err != nil {
		return fmt.Errorf("validate presubmit-periodic relationships: %w", err)
	}
	return nil
}

func exactReleasePresubmitKey(name, release string) (string, bool) {
	prefix := "pull-ci-openshift-hypershift-release-" + release + "-"
	value, found := strings.CutPrefix(name, prefix)
	return release + "/" + value, found && value != ""
}

func exactReleasePeriodicKey(name, release string) (string, bool) {
	prefix := "periodic-ci-openshift-hypershift-release-" + release + "-periodics-"
	value, found := strings.CutPrefix(name, prefix)
	return release + "/" + value, found && value != ""
}

func validatePresubmitPeriodicRelationships(registry *Registry, index map[string]*Job) error {
	seen := make(map[string]struct{}, len(registry.PresubmitPeriodicRelationships))
	periodicOwners := make(map[string]string)
	for _, relationship := range registry.PresubmitPeriodicRelationships {
		if relationship.PresubmitID == "" || len(relationship.PeriodicIDs) == 0 || relationship.Basis == "" {
			return fmt.Errorf("relationship for presubmit %q is incomplete", relationship.PresubmitID)
		}
		if !relationship.Provisional {
			return fmt.Errorf("relationship for presubmit %q is not marked provisional", relationship.PresubmitID)
		}
		if relationship.Basis != PresubmitPeriodicRelationshipBasisExactReleaseName && relationship.Basis != PresubmitPeriodicRelationshipBasisManualOverride {
			return fmt.Errorf("relationship for presubmit %q has unsupported basis %q", relationship.PresubmitID, relationship.Basis)
		}
		if _, found := seen[relationship.PresubmitID]; found {
			return fmt.Errorf("duplicate relationship for presubmit %q", relationship.PresubmitID)
		}
		seen[relationship.PresubmitID] = struct{}{}
		presubmit := index[relationship.PresubmitID]
		if presubmit == nil || presubmit.Type != "presubmit" {
			return fmt.Errorf("relationship source %q is not a presubmit job", relationship.PresubmitID)
		}
		seenPeriodics := make(map[string]struct{}, len(relationship.PeriodicIDs))
		for _, id := range relationship.PeriodicIDs {
			if _, found := seenPeriodics[id]; found {
				return fmt.Errorf("relationship for presubmit %q contains duplicate periodic %q", relationship.PresubmitID, id)
			}
			seenPeriodics[id] = struct{}{}
			if owner, found := periodicOwners[id]; found {
				return fmt.Errorf("periodic %q is related to both presubmits %q and %q", id, owner, relationship.PresubmitID)
			}
			periodicOwners[id] = relationship.PresubmitID
			if periodic := index[id]; periodic == nil || periodic.Type != "periodic" {
				return fmt.Errorf("relationship target %q is not a periodic job", id)
			}
		}
	}
	return nil
}
