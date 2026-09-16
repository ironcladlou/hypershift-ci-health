package jobregistry

import (
	"fmt"
	"sort"
	"strings"
)

// developmentRelease is the release line tested by main-branch presubmits.
// Until Prow expresses this relationship, changing the development line is a
// single explicit policy update here.
const developmentRelease = "5.1"

// periodicScenarioAliases contains only differences between periodic and
// presubmit scenario names. Branch and release selection are handled by the
// general policy rather than by per-job relationship overrides.
var periodicScenarioAliases = map[string]string{
	"e2e-aws-ovn":              "e2e-aws",
	"e2e-aws-upgrade":          "e2e-aws-upgrade-hypershift-operator",
	"e2e-kubevirt-aws-ovn-csi": "e2e-kubevirt-aws-ovn-reduced",
	"e2e-aws-ovn-conformance":  "e2e-conformance",
}

func populatePeriodicCounterparts(registry *Registry) error {
	index := registry.Index()
	for i := range registry.Jobs {
		if registry.Jobs[i].Presubmit != nil {
			registry.Jobs[i].Presubmit.PeriodicCounterparts = []PeriodicCounterpart{}
		}
	}

	for i := range registry.Jobs {
		periodic := &registry.Jobs[i]
		if periodic.Type != "periodic" || len(periodic.Versions) != 1 {
			continue
		}
		release := periodic.Versions[0]
		periodicScenario, ok := periodicScenarioName(periodic.Name, release)
		if !ok {
			continue
		}

		presubmitScenario := periodicScenario
		basis := PeriodicCounterpartBasisReleaseBranchPolicy
		if alias, found := periodicScenarioAliases[periodicScenario]; found {
			presubmitScenario = alias
			basis = PeriodicCounterpartBasisReleaseBranchPolicyWithAlias
		}

		branch := "release-" + release
		if release == developmentRelease {
			branch = "main"
		}
		presubmitID := "pull-ci-openshift-hypershift-" + branch + "-" + presubmitScenario
		presubmit := index[presubmitID]
		if presubmit == nil || presubmit.Type != "presubmit" || presubmit.Presubmit == nil {
			continue
		}

		description := fmt.Sprintf("Matched the %s periodic scenario %q to the %s presubmit using the release branch policy.", release, periodicScenario, branch)
		if periodicScenario != presubmitScenario {
			description = fmt.Sprintf("Matched the %s periodic scenario %q to presubmit scenario %q on %s using the release branch policy and a configured name alias.", release, periodicScenario, presubmitScenario, branch)
		}
		presubmit.Presubmit.PeriodicCounterparts = append(presubmit.Presubmit.PeriodicCounterparts, PeriodicCounterpart{
			JobID:       periodic.ID,
			Release:     release,
			Provisional: true,
			Basis:       basis,
			Description: description,
		})
	}

	for i := range registry.Jobs {
		if registry.Jobs[i].Presubmit == nil {
			continue
		}
		sort.Slice(registry.Jobs[i].Presubmit.PeriodicCounterparts, func(a, b int) bool {
			return registry.Jobs[i].Presubmit.PeriodicCounterparts[a].JobID < registry.Jobs[i].Presubmit.PeriodicCounterparts[b].JobID
		})
	}
	if err := validatePeriodicCounterparts(registry, index); err != nil {
		return fmt.Errorf("validate periodic counterparts: %w", err)
	}
	return nil
}

func periodicScenarioName(name, release string) (string, bool) {
	prefix := "periodic-ci-openshift-hypershift-release-" + release + "-periodics-"
	value, found := strings.CutPrefix(name, prefix)
	return value, found && value != ""
}

func validatePeriodicCounterparts(registry *Registry, index map[string]*Job) error {
	periodicOwners := make(map[string]string)
	for i := range registry.Jobs {
		presubmit := &registry.Jobs[i]
		if presubmit.Presubmit == nil {
			continue
		}
		seen := make(map[string]struct{}, len(presubmit.Presubmit.PeriodicCounterparts))
		for _, counterpart := range presubmit.Presubmit.PeriodicCounterparts {
			if counterpart.JobID == "" || counterpart.Release == "" || counterpart.Basis == "" || counterpart.Description == "" {
				return fmt.Errorf("presubmit %q has an incomplete periodic counterpart", presubmit.ID)
			}
			if !counterpart.Provisional {
				return fmt.Errorf("periodic counterpart %q on presubmit %q is not marked provisional", counterpart.JobID, presubmit.ID)
			}
			if counterpart.Basis != PeriodicCounterpartBasisReleaseBranchPolicy && counterpart.Basis != PeriodicCounterpartBasisReleaseBranchPolicyWithAlias {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has unsupported basis %q", counterpart.JobID, presubmit.ID, counterpart.Basis)
			}
			if _, found := seen[counterpart.JobID]; found {
				return fmt.Errorf("presubmit %q contains duplicate periodic counterpart %q", presubmit.ID, counterpart.JobID)
			}
			seen[counterpart.JobID] = struct{}{}
			if owner, found := periodicOwners[counterpart.JobID]; found {
				return fmt.Errorf("periodic %q is a counterpart of both presubmits %q and %q", counterpart.JobID, owner, presubmit.ID)
			}
			periodicOwners[counterpart.JobID] = presubmit.ID
			periodic := index[counterpart.JobID]
			if periodic == nil || periodic.Type != "periodic" {
				return fmt.Errorf("counterpart %q on presubmit %q is not a periodic job", counterpart.JobID, presubmit.ID)
			}
			if len(periodic.Versions) != 1 || periodic.Versions[0] != counterpart.Release {
				return fmt.Errorf("counterpart %q on presubmit %q does not uniquely test release %q", counterpart.JobID, presubmit.ID, counterpart.Release)
			}
		}
	}
	return nil
}
