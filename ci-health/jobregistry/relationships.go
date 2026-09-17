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
	proposalRationale  = "Proposed by applying a human-verified main-branch scenario mapping to jobs on the same release branch; requires human review."
)

type presubmitPolicy struct {
	developmentBranch           string
	developmentRelease          string
	sippyReleaseBranchAllowlist []string
}

var defaultPresubmitPolicy = presubmitPolicy{
	developmentBranch:           developmentBranch,
	developmentRelease:          developmentRelease,
	sippyReleaseBranchAllowlist: []string{},
}

// TODO: Publish structured, per-field derivation and metadata-gap records so
// consumers can audit registry inferences and automate upstream gap analysis.
// Keep generator policy private; expose the evidence, method, authority, and
// resolution state of derived facts instead.

func (p presubmitPolicy) validate() error {
	if p.developmentBranch == "" || p.developmentRelease == "" {
		return fmt.Errorf("development branch and release are required")
	}
	if !sort.StringsAreSorted(p.sippyReleaseBranchAllowlist) {
		return fmt.Errorf("Sippy release branch allowlist is not sorted")
	}
	for i, release := range p.sippyReleaseBranchAllowlist {
		if release == "" || i > 0 && release == p.sippyReleaseBranchAllowlist[i-1] {
			return fmt.Errorf("Sippy release branch allowlist contains an empty or duplicate release")
		}
	}
	return nil
}

var presubmitReleaseBranchRE = regexp.MustCompile(`^release-(\d+\.\d+)-(.+)$`)

type periodicMapping struct {
	PresubmitID string
	PeriodicID  string
}

// The registry publishes only relationships explicitly listed below. Manual
// mappings have been reviewed against job definitions; proposals are visible
// candidates that still require that review.
var manualPeriodicMappings = []periodicMapping{
	{"pull-ci-openshift-hypershift-main-e2e-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn"},
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
	{"pull-ci-openshift-hypershift-release-4.15-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.15-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws"},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-kubevirt-aws-ovn-csi"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-aws", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-aws"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-aks", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aks"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-azure-self-managed", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-azure-self-managed"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-gke", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-gke"},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-kubevirt-aws-ovn-csi"},
}

var proposedPeriodicMappings = []periodicMapping{}

func populatePeriodicCounterparts(registry *Registry, policy presubmitPolicy) error {
	index := registry.Index()
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Presubmit == nil {
			continue
		}
		job.Presubmit.PeriodicCounterparts = []PeriodicCounterpart{}
		branch, release, _, ok := presubmitTarget(job.Name, policy)
		if ok {
			job.Presubmit.TargetBranch = branch
			job.Presubmit.TargetRelease = release
		}
	}

	mappingSets := []struct {
		name         string
		mappings     []periodicMapping
		source       PeriodicCounterpartSource
		verification PeriodicCounterpartVerification
		rationale    string
	}{
		{"manual", manualPeriodicMappings, PeriodicCounterpartSourceRegistryManual, PeriodicCounterpartVerificationHuman, manualRationale},
		{"proposal", proposedPeriodicMappings, PeriodicCounterpartSourceRegistryProposal, PeriodicCounterpartVerificationNeedsReview, proposalRationale},
	}
	for _, set := range mappingSets {
		for _, mapping := range set.mappings {
			presubmit := index[mapping.PresubmitID]
			if presubmit == nil || presubmit.Type != "presubmit" || presubmit.Presubmit == nil {
				return fmt.Errorf("%s mapping presubmit %q is not a presubmit job", set.name, mapping.PresubmitID)
			}
			periodic := index[mapping.PeriodicID]
			if periodic == nil || periodic.Type != "periodic" || len(periodic.Versions) != 1 {
				return fmt.Errorf("%s mapping periodic %q is not a single-release periodic job", set.name, mapping.PeriodicID)
			}
			presubmit.Presubmit.PeriodicCounterparts = append(presubmit.Presubmit.PeriodicCounterparts, PeriodicCounterpart{
				JobID:         periodic.ID,
				TestedRelease: periodic.Versions[0],
				Source:        set.source,
				Verification:  set.verification,
				Rationale:     set.rationale,
			})
		}
	}

	for i := range registry.Jobs {
		if registry.Jobs[i].Presubmit == nil {
			continue
		}
		sort.Slice(registry.Jobs[i].Presubmit.PeriodicCounterparts, func(a, b int) bool {
			return registry.Jobs[i].Presubmit.PeriodicCounterparts[a].JobID < registry.Jobs[i].Presubmit.PeriodicCounterparts[b].JobID
		})
	}
	return nil
}

func presubmitTarget(name string, policy presubmitPolicy) (branch, release, scenario string, ok bool) {
	const prefix = "pull-ci-openshift-hypershift-"
	value, found := strings.CutPrefix(name, prefix)
	if !found {
		return "", "", "", false
	}
	if scenario, found = strings.CutPrefix(value, policy.developmentBranch+"-"); found && scenario != "" {
		return policy.developmentBranch, policy.developmentRelease, scenario, true
	}
	match := presubmitReleaseBranchRE.FindStringSubmatch(value)
	if match == nil {
		return "", "", "", false
	}
	return "release-" + match[1], match[1], match[2], true
}

func validatePeriodicCounterparts(registry *Registry, index map[string]*Job) error {
	for i := range registry.Jobs {
		presubmit := &registry.Jobs[i]
		if presubmit.Presubmit == nil {
			continue
		}
		if presubmit.Presubmit.SippyIngestion.Basis == "" {
			return fmt.Errorf("presubmit %q has no Sippy ingestion basis", presubmit.ID)
		}
		seen := make(map[string]struct{}, len(presubmit.Presubmit.PeriodicCounterparts))
		for _, counterpart := range presubmit.Presubmit.PeriodicCounterparts {
			if presubmit.Presubmit.TargetBranch == "" || presubmit.Presubmit.TargetRelease == "" {
				return fmt.Errorf("presubmit %q with a periodic counterpart has no target branch or release", presubmit.ID)
			}
			if counterpart.JobID == "" || counterpart.TestedRelease == "" || counterpart.Source == "" || counterpart.Verification == "" || counterpart.Rationale == "" {
				return fmt.Errorf("presubmit %q has an incomplete periodic counterpart", presubmit.ID)
			}
			if counterpart.Source != PeriodicCounterpartSourceRegistryManual && counterpart.Source != PeriodicCounterpartSourceRegistryProposal {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has unsupported source %q", counterpart.JobID, presubmit.ID, counterpart.Source)
			}
			if counterpart.Verification != PeriodicCounterpartVerificationHuman && counterpart.Verification != PeriodicCounterpartVerificationNeedsReview {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has unsupported verification %q", counterpart.JobID, presubmit.ID, counterpart.Verification)
			}
			if counterpart.Source == PeriodicCounterpartSourceRegistryManual && counterpart.Verification != PeriodicCounterpartVerificationHuman ||
				counterpart.Source == PeriodicCounterpartSourceRegistryProposal && counterpart.Verification != PeriodicCounterpartVerificationNeedsReview {
				return fmt.Errorf("periodic counterpart %q on presubmit %q has inconsistent source %q and verification %q", counterpart.JobID, presubmit.ID, counterpart.Source, counterpart.Verification)
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
