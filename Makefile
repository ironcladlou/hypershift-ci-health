.PHONY: upstream-references

upstream-references:
	git clone --depth 1 --single-branch https://github.com/openshift/release.git .upstreams/openshift-release 2>/dev/null || git -C .upstreams/openshift-release pull --ff-only
	git clone --depth 1 --single-branch https://github.com/openshift/sippy.git .upstreams/sippy 2>/dev/null || git -C .upstreams/sippy pull --ff-only
