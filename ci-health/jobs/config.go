package jobs

type Platform string

const (
	PlatformAWS      Platform = "AWS"
	PlatformAzure    Platform = "Azure"
	PlatformGKE      Platform = "GKE"
	PlatformKubeVirt Platform = "KubeVirt"
)

type PeriodicJobConfig struct {
	Name        string
	ProwJobName string
	Release     string
}

type BlockingJobConfig struct {
	Name        string
	ProwJobName string
	Platform    Platform
	Periodics   []PeriodicJobConfig
}

var BlockingJobs = []BlockingJobConfig{
	{
		Name:        "e2e-aws",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws",
		Platform:    PlatformAWS,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-aws-ovn", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn", Release: "5.0"},
		},
	},
	{
		Name:        "e2e-aks",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aks",
		Platform:    PlatformAzure,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-aks", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aks", Release: "5.0"},
		},
	},
	{
		Name:        "e2e-v2-azure-self-managed",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-v2-azure-self-managed",
		Platform:    PlatformAzure,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-v2-azure-self-managed", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-azure-self-managed", Release: "5.0"},
		},
	},
	{
		Name:        "e2e-aws-upgrade-hypershift-operator",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator",
		Platform:    PlatformAWS,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-aws-upgrade", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-upgrade", Release: "5.0"},
		},
	},
	{
		Name:        "e2e-v2-gke",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-v2-gke",
		Platform:    PlatformGKE,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-v2-gke", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-gke", Release: "5.0"},
		},
	},
	{
		Name:        "e2e-aws-4-22",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws-4-22",
		Platform:    PlatformAWS,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-aws-ovn", ProwJobName: "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aws-ovn", Release: "4.22"},
		},
	},
	{
		Name:        "e2e-aks-4-22",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aks-4-22",
		Platform:    PlatformAzure,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-aks", ProwJobName: "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks", Release: "4.22"},
		},
	},
	{
		Name:        "e2e-kubevirt-aws-ovn-reduced",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced",
		Platform:    PlatformKubeVirt,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-kubevirt-aws-ovn-csi", ProwJobName: "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-kubevirt-aws-ovn-csi", Release: "4.22"},
		},
	},
	{
		Name:        "e2e-v2-aws",
		ProwJobName: "pull-ci-openshift-hypershift-main-e2e-v2-aws",
		Platform:    PlatformAWS,
		Periodics: []PeriodicJobConfig{
			{Name: "e2e-v2-aws", ProwJobName: "periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-v2-aws", Release: "5.0"},
			{Name: "e2e-v2-aws", ProwJobName: "periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-v2-aws", Release: "4.22"},
		},
	},
}

// Releases returns the unique set of release versions referenced by periodic jobs.
func Releases() []string {
	seen := map[string]bool{}
	var releases []string
	for _, job := range BlockingJobs {
		for _, p := range job.Periodics {
			if !seen[p.Release] {
				seen[p.Release] = true
				releases = append(releases, p.Release)
			}
		}
	}
	return releases
}

// Platforms returns the unique set of platforms referenced by blocking jobs.
func Platforms() []string {
	seen := map[Platform]bool{}
	var platforms []string
	for _, job := range BlockingJobs {
		if !seen[job.Platform] {
			seen[job.Platform] = true
			platforms = append(platforms, string(job.Platform))
		}
	}
	return platforms
}

// PresubmitProwJobNames returns the prow job names for all blocking presubmits.
func PresubmitProwJobNames() []string {
	names := make([]string, len(BlockingJobs))
	for i, job := range BlockingJobs {
		names[i] = job.ProwJobName
	}
	return names
}

// PeriodicProwJobNamesByRelease returns periodic prow job names grouped by release.
func PeriodicProwJobNamesByRelease() map[string][]string {
	result := make(map[string][]string)
	for _, job := range BlockingJobs {
		for _, p := range job.Periodics {
			result[p.Release] = append(result[p.Release], p.ProwJobName)
		}
	}
	return result
}
