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
	Notes       string
}

// The registry publishes only the verified relationships explicitly listed below.
var verifiedPeriodicMappings = []periodicMapping{
	{"pull-ci-openshift-hypershift-main-e2e-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-main-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws", ""},
	{"pull-ci-openshift-hypershift-main-e2e-aks", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-main-e2e-v2-azure-self-managed", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-azure-self-managed", ""},
	{"pull-ci-openshift-hypershift-main-e2e-v2-gke", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-gke", ""},
	{"pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-aws", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-aks", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-aws", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-aks", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-aws", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-aks", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-aws", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-aks", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn-conformance", ""},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-aws", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-aws-ovn-conformance", ""},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-aws", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-aws-ovn-conformance", ""},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-aws", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-conformance", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance", ""},
	{"pull-ci-openshift-hypershift-release-4.15-e2e-aws", "periodic-ci-openshift-hypershift-release-4.15-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.14-e2e-aws", "periodic-ci-openshift-hypershift-release-4.14-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-4.15-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.15-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.16-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.17-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.17-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.18-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.18-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.19-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.20-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.20-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.21-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.21-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws", ""},
	{"pull-ci-openshift-hypershift-release-4.22-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-aws", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-aws", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-aws", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-aks", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aks", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-azure-self-managed", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-azure-self-managed", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-v2-gke", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-gke", ""},
	{"pull-ci-openshift-hypershift-release-5.0-e2e-kubevirt-aws-ovn-reduced", "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-kubevirt-aws-ovn-csi", ""},
	{"pull-ci-openshift-hypershift-main-e2e-aks-managed-hsm", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks-managed-hsm", "Matching Azure AKS workflow, cluster profile, and managed-HSM configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-aro-hcp", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aro-hcp", "Matching ARO HCP workflow and scenario configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-aws-autonode", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-autonode", "Matching nested AWS workflow, dependencies, and AutoNode configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-aws-external-oidc-techpreview", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-external-oidc-techpreview", "Matching external-OIDC workflow and tech-preview configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-aws-ovn-conformance-ccm", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance-ccm", "Matching AWS conformance workflow and scenario configuration; the periodic runs monthly."},
	{"pull-ci-openshift-hypershift-main-e2e-aws-ovn-conformance-techpreview", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance-techpreview", "Matching AWS conformance workflow and guest feature set; the periodic additionally skips two NLB tests: internal reachability with hairpinning traffic and reachability with target-node-labels."},
	{"pull-ci-openshift-hypershift-main-e2e-azure-aks-external-oidc-techpreview", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-azure-aks-external-oidc-techpreview", "Matching Azure external-OIDC workflow and tech-preview configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-azure-aks-ovn-conformance", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-azure-aks-ovn-conformance", "Matching Azure conformance workflow; the periodic clears the marketplace image publisher, offer, SKU, and version settings."},
	{"pull-ci-openshift-hypershift-main-e2e-azure-aks-ovn-conformance-fips", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-azure-aks-ovn-conformance-fips", "Matching Azure conformance workflow, FIPS setting, test suite, and skips; the periodic clears the marketplace image publisher, offer, SKU, and version settings."},
	{"pull-ci-openshift-hypershift-main-e2e-azure-kubevirt-ovn", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-azure-kubevirt-ovn", "Matching Azure KubeVirt conformance workflow and test suite. The presubmit pins the management-cluster installer image to latest-50, sets two HyperShift nodes, and uses ODF channel stable-4.20; the periodic uses prerelease ODF catalog 5.0 without those image and node-count overrides."},
	{"pull-ci-openshift-hypershift-main-e2e-conformance", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance", "Different CI target names select matching AWS conformance workflows, cluster profiles, and scenario configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-conformance-fips", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance-fips", "Different CI target names select matching AWS conformance workflows, cluster profiles, and FIPS configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-openstack-aws", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-openstack-aws", "Matching nested AWS workflow and OpenStack E2E test chain; only the presubmit explicitly adds OpenStack gathering, nested management-cluster cleanup, and a six-hour timeout."},
	{"pull-ci-openshift-hypershift-main-e2e-openstack-aws-conformance", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-openstack-aws-conformance", "Matching OpenStack conformance workflow and scenario configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-openstack-aws-csi-cinder", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-openstack-aws-csi-cinder", "Matching OpenStack conformance workflow, CSI test suite, and Cinder driver manifest."},
	{"pull-ci-openshift-hypershift-main-e2e-openstack-aws-csi-manila", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-openstack-aws-csi-manila", "Matching OpenStack conformance workflow, CSI test suite, and Manila driver manifest."},
	{"pull-ci-openshift-hypershift-main-e2e-openstack-aws-nfv", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-openstack-aws-nfv", "Matching OpenStack NFV workflow and scenario configuration."},
	{"pull-ci-openshift-hypershift-main-e2e-rosa-hyperfleet", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-rosa-hyperfleet", "Matching ephemeral Hyperfleet E2E workflow. The presubmit injects PR-built operator images and uses candidate releases; the periodic uses published images and nightly releases, explicitly provisions the cluster, and waits for teardown."},
	{"pull-ci-openshift-hypershift-main-e2e-v2-aws-backuprestore", "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-v2-aws-backuprestore", "Matching AWS backup/restore workflow and scenario configuration; the periodic runs weekly."},
}

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

	for _, mapping := range verifiedPeriodicMappings {
		presubmit := index[mapping.PresubmitID]
		periodic := index[mapping.PeriodicID]
		// A selected source snapshot may predate either job in a verified pair.
		if presubmit == nil || periodic == nil {
			continue
		}
		if presubmit.Type != JobTypePresubmit || presubmit.Presubmit == nil {
			return fmt.Errorf("verified mapping presubmit %q is not a presubmit job", mapping.PresubmitID)
		}
		if periodic.Type != JobTypePeriodic || len(periodic.Versions) != 1 {
			return fmt.Errorf("verified mapping periodic %q is not a single-release periodic job", mapping.PeriodicID)
		}
		presubmit.Presubmit.PeriodicCounterparts = append(presubmit.Presubmit.PeriodicCounterparts, PeriodicCounterpart{
			JobID:         periodic.ID,
			TestedRelease: periodic.Versions[0],
			Source:        PeriodicCounterpartSourceRegistryManual,
			Verification:  PeriodicCounterpartVerificationHuman,
			Rationale:     manualRationale,
			Notes:         mapping.Notes,
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
