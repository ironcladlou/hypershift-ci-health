package jobregistry

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	developmentBranch  = "main"
	developmentRelease = "5.1"
	manualRationale    = "A human reviewed the job definitions and verified that these jobs exercise corresponding test scenarios."
)

var presubmitReleaseBranchRE = regexp.MustCompile(`^release-(\d+\.\d+)-(.+)$`)

type manualPeriodicMapping struct {
	PresubmitID string
	PeriodicID  string
}

// manualPeriodicMappings is the sole source of presubmit/periodic
// relationships published by the registry. Heuristic candidates must not be
// added here until a human has compared the underlying job definitions.
var manualPeriodicMappings = []manualPeriodicMapping{
	{"pull-ci-openshift-hypershift-main-e2e-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-upgrade"},
	{"pull-ci-openshift-hypershift-main-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws"},
	{"pull-ci-openshift-hypershift-main-e2e-aks", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-main-e2e-v2-azure-self-managed", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-azure-self-managed"},
	{"pull-ci-openshift-hypershift-main-e2e-v2-gke", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-gke"},
	{"pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-aws", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-aks", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-aws", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-aks", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-aws", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-aks", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-aws", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-aks", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn-conformance"},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-aws", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn-conformance"},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-aws", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn-conformance"},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-aws", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance"},
	{"pull-ci-openshift-hypershift-release-4.15-e2e-aws", "periodic-ci-openshift-hypershift-release-4.15-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-4.14-e2e-aws", "periodic-ci-openshift-hypershift-release-4.14-periodics-e2e-aws-ovn"},
}

func populatePeriodicCounterparts(registry *Registry) error {
	index := registry.Index()
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Presubmit == nil {
			continue
		}
		job.Presubmit.PeriodicCounterparts = []PeriodicCounterpart{}
		branch, release, _, ok := presubmitTarget(job.Name, registry.PresubmitPolicy)
		if ok {
			job.Presubmit.TargetBranch = branch
			job.Presubmit.TargetRelease = release
		}
	}

	for _, mapping := range manualPeriodicMappings {
		presubmit := index[mapping.PresubmitID]
		if presubmit == nil || presubmit.Type != "presubmit" || presubmit.Presubmit == nil {
			return fmt.Errorf("manual mapping presubmit %q is not a presubmit job", mapping.PresubmitID)
		}
		periodic := index[mapping.PeriodicID]
		if periodic == nil || periodic.Type != "periodic" || len(periodic.Versions) != 1 {
			return fmt.Errorf("manual mapping periodic %q is not a single-release periodic job", mapping.PeriodicID)
		}
		presubmit.Presubmit.PeriodicCounterparts = append(presubmit.Presubmit.PeriodicCounterparts, PeriodicCounterpart{
			JobID:         periodic.ID,
			TestedRelease: periodic.Versions[0],
			Source:        PeriodicCounterpartSourceRegistryManual,
			Verification:  PeriodicCounterpartVerificationHuman,
			Rationale:     manualRationale,
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

func presubmitTarget(name string, policy PresubmitPolicy) (branch, release, scenario string, ok bool) {
	const prefix = "pull-ci-openshift-hypershift-"
	value, found := strings.CutPrefix(name, prefix)
	if !found {
		return "", "", "", false
	}
	if scenario, found = strings.CutPrefix(value, policy.DevelopmentBranch+"-"); found && scenario != "" {
		return policy.DevelopmentBranch, policy.DevelopmentRelease, scenario, true
	}
	match := presubmitReleaseBranchRE.FindStringSubmatch(value)
	if match == nil {
		return "", "", "", false
	}
	return "release-" + match[1], match[1], match[2], true
}

func validatePeriodicCounterparts(registry *Registry, index map[string]*Job) error {
	if registry.PresubmitPolicy.DevelopmentBranch == "" || registry.PresubmitPolicy.DevelopmentRelease == "" || registry.PresubmitPolicy.Description == "" {
		return fmt.Errorf("presubmit policy is incomplete")
	}
	if !registry.PresubmitPolicy.Provisional {
		return fmt.Errorf("presubmit policy is not marked provisional")
	}
	for i := range registry.Jobs {
		presubmit := &registry.Jobs[i]
		if presubmit.Presubmit == nil {
			continue
		}
		seen := make(map[string]struct{}, len(presubmit.Presubmit.PeriodicCounterparts))
		for _, counterpart := range presubmit.Presubmit.PeriodicCounterparts {
			if presubmit.Presubmit.TargetBranch == "" || presubmit.Presubmit.TargetRelease == "" {
				return fmt.Errorf("presubmit %q with a periodic counterpart has no target branch or release", presubmit.ID)
			}
			if counterpart.JobID == "" || counterpart.TestedRelease == "" || counterpart.Source == "" || counterpart.Verification == "" || counterpart.Rationale == "" {
				return fmt.Errorf("presubmit %q has an incomplete periodic counterpart", presubmit.ID)
			}
			if counterpart.Source != PeriodicCounterpartSourceRegistryManual {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has unsupported source %q", counterpart.JobID, presubmit.ID, counterpart.Source)
			}
			if counterpart.Verification != PeriodicCounterpartVerificationHuman {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has unsupported verification %q", counterpart.JobID, presubmit.ID, counterpart.Verification)
			}
			if _, found := seen[counterpart.JobID]; found {
				return fmt.Errorf("presubmit %q contains duplicate periodic counterpart %q", presubmit.ID, counterpart.JobID)
			}
			seen[counterpart.JobID] = struct{}{}
			periodic := index[counterpart.JobID]
			if periodic == nil || periodic.Type != "periodic" {
				return fmt.Errorf("counterpart %q on presubmit %q is not a periodic job", counterpart.JobID, presubmit.ID)
			}
			if len(periodic.Versions) != 1 || periodic.Versions[0] != counterpart.TestedRelease {
				return fmt.Errorf("counterpart %q on presubmit %q does not uniquely test release %q", counterpart.JobID, presubmit.ID, counterpart.TestedRelease)
			}
		}
	}
	return nil
}
