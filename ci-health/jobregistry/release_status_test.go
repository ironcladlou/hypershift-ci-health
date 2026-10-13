package jobregistry

import "testing"

func TestReleaseStatusRouting(t *testing.T) {
	for _, test := range []struct {
		name, base, stream, architecture, want string
	}{
		{"hypershift", defaultReleaseStatusURL, "hypershift", "", "https://hypershift-release.apps.ci.l2s4.p1.openshiftapps.com/releasestream/hypershift"},
		{"ocp-amd64", defaultReleaseStatusURL, "5.1.0-0.ci", "amd64", "https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/releasestream/5.1.0-0.ci"},
		{"ocp-arm64", defaultReleaseStatusURL, "5.1.0-0.nightly-arm64", "arm64", "https://arm64.ocp.releases.ci.openshift.org/releasestream/5.1.0-0.nightly-arm64"},
		{"hypershift-override", "https://example.test/", "hypershift", "", "https://example.test/releasestream/hypershift"},
		{"ocp-override", "https://example.test", "5.1.0-0.nightly-arm64", "arm64", "https://example.test/releasestream/5.1.0-0.nightly-arm64"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := releaseStatusURL(test.base, test.stream, test.architecture); got != test.want {
				t.Fatalf("release status URL = %q, want %q", got, test.want)
			}
		})
	}
}
