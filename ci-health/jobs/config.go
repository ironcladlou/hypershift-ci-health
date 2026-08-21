package jobs

import (
	"fmt"
	"strconv"
	"strings"
)

type Platform string

const (
	PlatformAWS      Platform = "AWS"
	PlatformAzure    Platform = "Azure"
	PlatformGKE      Platform = "GKE"
	PlatformKubeVirt Platform = "KubeVirt"
)

const FutureRelease = "5.1"

type Role string

const (
	RoleFuture  Role = "future"
	RoleNMinus1 Role = "n-1"
)

type JobSpec struct {
	Name         string
	Platform     Platform
	PeriodicName string // periodic short name if different from presubmit Name
	HasNMinus1   bool
}

var Jobs = []JobSpec{
	{Name: "e2e-aws", Platform: PlatformAWS, PeriodicName: "e2e-aws-ovn", HasNMinus1: true},
	{Name: "e2e-aws-upgrade-hypershift-operator", Platform: PlatformAWS, PeriodicName: "e2e-aws-upgrade"},
	{Name: "e2e-v2-aws", Platform: PlatformAWS},
	{Name: "e2e-aks", Platform: PlatformAzure, HasNMinus1: true},
	{Name: "e2e-v2-azure-self-managed", Platform: PlatformAzure},
	{Name: "e2e-v2-gke", Platform: PlatformGKE},
	{Name: "e2e-kubevirt-aws-ovn-reduced", Platform: PlatformKubeVirt, PeriodicName: "e2e-kubevirt-aws-ovn-csi"},
}

func CurrentRelease() string {
	parts := strings.Split(FutureRelease, ".")
	if len(parts) != 2 {
		panic("invalid FutureRelease: " + FutureRelease)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor <= 0 {
		panic("cannot compute N-1 for FutureRelease: " + FutureRelease)
	}
	return parts[0] + "." + strconv.Itoa(minor-1)
}

func versionSuffix(release string) string {
	return strings.ReplaceAll(release, ".", "-")
}

func presubmitProwName(name string) string {
	return "pull-ci-openshift-hypershift-main-" + name
}

func periodicProwName(periodicName, release string) string {
	return fmt.Sprintf("periodic-ci-openshift-hypershift-release-%s-periodics-%s", release, periodicName)
}

func RoleLabel(role Role, release string) string {
	if role == RoleNMinus1 {
		return fmt.Sprintf("N-1 (%s)", release)
	}
	return fmt.Sprintf("Future (%s)", release)
}

type PeriodicJobConfig struct {
	Name        string
	ProwJobName string
	Release     string
}

type BlockingJobConfig struct {
	Name        string
	ProwJobName string
	Platform    Platform
	Role        Role
	Periodics   []PeriodicJobConfig
}

var BlockingJobs []BlockingJobConfig

func init() {
	current := CurrentRelease()

	for _, spec := range Jobs {
		periodicName := spec.PeriodicName
		if periodicName == "" {
			periodicName = spec.Name
		}

		BlockingJobs = append(BlockingJobs, BlockingJobConfig{
			Name:        spec.Name,
			ProwJobName: presubmitProwName(spec.Name),
			Platform:    spec.Platform,
			Role:        RoleFuture,
			Periodics: []PeriodicJobConfig{
				{Name: periodicName, ProwJobName: periodicProwName(periodicName, FutureRelease), Release: FutureRelease},
			},
		})

		if spec.HasNMinus1 {
			nm1Name := spec.Name + "-" + versionSuffix(current)
			BlockingJobs = append(BlockingJobs, BlockingJobConfig{
				Name:        nm1Name,
				ProwJobName: presubmitProwName(nm1Name),
				Platform:    spec.Platform,
				Role:        RoleNMinus1,
				Periodics: []PeriodicJobConfig{
					{Name: periodicName, ProwJobName: periodicProwName(periodicName, current), Release: current},
				},
			})
		}
	}
}

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

func PresubmitProwJobNames() []string {
	names := make([]string, len(BlockingJobs))
	for i, job := range BlockingJobs {
		names[i] = job.ProwJobName
	}
	return names
}

func PeriodicProwJobNamesByRelease() map[string][]string {
	result := make(map[string][]string)
	for _, job := range BlockingJobs {
		for _, p := range job.Periodics {
			result[p.Release] = append(result[p.Release], p.ProwJobName)
		}
	}
	return result
}
