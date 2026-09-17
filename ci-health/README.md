# HyperShift CI Health

A dashboard for the health of HyperShift's required presubmit jobs, release
payload blockers, and Component Readiness blockers. Its data pipeline produces
four immutable, versioned artifacts: registry facts, a report plan, Sippy
observations, and an evaluated health report. The HTTP server performs no live
collection.

## Quick start

Requires Go 1.25+ and a checkout of
[`openshift/release`](https://github.com/openshift/release).

Generate the artifacts and start the server:

```bash
make generate-artifacts RELEASE_DIR=/path/to/openshift-release
./bin/ci-health serve
```

The server reads the generated files from its current directory. File paths can
be overridden with command-line flags.

## Data pipeline

```text
openshift/release -> job registry
job registry + selection policy -> report plan
report plan + Sippy -> Sippy observation
registry + plan + observation -> health report
```

The registry is the reusable job inventory. The report plan selects the jobs to
analyze. Sippy is authoritative for Component Readiness participation and job
health evidence. The health report combines those inputs for presentation.

Every downstream artifact carries a digest of its inputs, so mismatched or stale
artifact sets fail validation instead of being served silently. Individual Sippy
request failures are recorded in the observation and produce a partial report;
successful results remain available.

To generate only the reusable registry:

```bash
make generate-job-registry RELEASE_DIR=/path/to/openshift-release
```

## Deployment

Deployment targets OpenShift and requires `oc` access to the target cluster:

```bash
make setup
make deploy
```

## Public Job Registry API

- `GET /api/job-registry` — complete generated job registry
- `GET /api/job-registry/jobs/{id}` — one job registry entry
- `GET /api/docs` — interactive Scalar API reference
- `GET /api/openapi.json` or `/api/openapi.yaml` — machine-readable OpenAPI description
- `GET /api/schemas/{schema}.json` — generated JSON Schema for an API resource

These endpoints form the supported external API and are described by the
published OpenAPI contract. Other HTTP endpoints are private implementation
details of the dashboard and carry no compatibility guarantee.
