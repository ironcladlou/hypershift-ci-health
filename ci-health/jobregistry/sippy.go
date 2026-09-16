package jobregistry

import "fmt"

// populateSippyIngestion decorates presubmits with the registry's intended
// Sippy availability. It does not probe Sippy or claim that an enabled job has
// produced data.
func populateSippyIngestion(registry *Registry) {
	allowedReleases := make(map[string]struct{}, len(registry.PresubmitPolicy.SippyReleaseBranchAllowlist))
	for _, release := range registry.PresubmitPolicy.SippyReleaseBranchAllowlist {
		allowedReleases[release] = struct{}{}
	}
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Presubmit == nil {
			continue
		}
		switch {
		case job.Presubmit.TargetBranch == registry.PresubmitPolicy.DevelopmentBranch:
			job.Presubmit.SippyIngestion = SippyIngestion{
				Enabled: true,
				Basis:   "Sippy presubmit ingestion is enabled for the development branch.",
			}
		case releaseAllowed(allowedReleases, job.Presubmit.TargetRelease):
			job.Presubmit.SippyIngestion = SippyIngestion{
				Enabled: true,
				Basis:   fmt.Sprintf("Sippy presubmit ingestion is enabled for release branch %s by registry policy.", job.Presubmit.TargetRelease),
			}
		case job.Presubmit.TargetRelease != "":
			job.Presubmit.SippyIngestion = SippyIngestion{
				Basis: fmt.Sprintf("Sippy presubmit ingestion is not enabled for release branch %s by registry policy.", job.Presubmit.TargetRelease),
			}
		default:
			job.Presubmit.SippyIngestion = SippyIngestion{
				Basis: "Sippy presubmit ingestion is disabled because no release branch policy applies to this job.",
			}
		}
	}
}

func releaseAllowed(allowedReleases map[string]struct{}, release string) bool {
	_, found := allowedReleases[release]
	return found
}
