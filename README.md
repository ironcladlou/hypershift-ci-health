# hypershift-ci-health

Tools and dashboards for HyperShift CI observability.

## Apps

Each app is self-contained with its own Makefile and deploy manifests.

**[ci-health](ci-health/)** — Merge queue health dashboard. Surfaces pass rates and failure patterns across blocking presubmit jobs using the Sippy API.

**[aws-resources](aws-resources/)** — E2E resource monitor. Discovers AWS resources tagged by e2e tests, correlates with Prow job status, and identifies orphans.

## Deploying

Requires `oc`.

```
cd ci-health && make deploy
cd aws-resources && make deploy
```

See each app's README for details.

## Reference repos

`./upstreams/sippy/` and `./upstreams/openshift-release/` are vendored source trees for reference during development and can be deployed with `make upstream-references`.
