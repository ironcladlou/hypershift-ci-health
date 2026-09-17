# HyperShift CI Health

A dashboard for the health of HyperShift's required presubmit jobs, release
payload blockers, and Component Readiness blockers. Its data pipeline produces
four immutable, versioned artifacts: registry facts, a report plan, Sippy
observations, and an evaluated health report. The HTTP server performs no live
collection.

## Local development

### Prerequisites

- Go 1.25+
- Generated registry, plan, observation, and report JSON artifacts
- `oc` for deployment

```bash
make build
./bin/ci-health serve --dev
```

`--dev` serves `index.html` from the filesystem for live editing. At startup,
the server reads `job-registry.json`, `report-plan.json`,
`sippy-observation.json`, and `health-report.json` from its current directory,
validates their provenance chain, and then serves cached data. Each path can be
overridden with its corresponding command-line flag.

## Data pipeline

```text
openshift/release -> job registry
job registry + selection policy -> report plan
report plan + Sippy -> Sippy observation
registry + plan + observation -> health report
```

The registry is the reusable inventory. The pointer-free report plan is the
deterministic, dashboard-specific selection and query plan. Sippy observations
own Component Readiness participation and run evidence. Health report evaluation
is a pure join; it performs no network or filesystem I/O.

Generate the complete pipeline with:

```bash
make generate-artifacts RELEASE_DIR=/path/to/openshift-release
```

Every downstream artifact carries a digest of its inputs, so mismatched or stale
artifact sets fail validation instead of being served silently. Individual Sippy
request failures are recorded in the observation and produce a partial report;
successful results remain available.

## Job registry command

The standalone command generates a registry from an `openshift/release`
checkout:

```bash
make generate-job-registry RELEASE_DIR=/path/to/openshift-release
```

The reusable registry API lives in `jobregistry`. Startup validates
every referenced ID and obtains all job metadata from the registry.

## Deployment

On every pod start, an init container makes a shallow, blob-filtered sparse
clone of `openshift/release` and generates the complete artifact chain in an
`emptyDir`. The server container only loads those files. An hourly CronJob
performs a rolling restart, and the previous pod keeps serving its cached report
until the replacement is ready. Only artifact generation needs outbound access
to GitHub and Sippy.

```bash
make setup
make deploy
```

## API

- `GET /` — dashboard UI
- `GET /api/health` — job health snapshot for the 1-week, 2-week, and 1-month windows
- `GET /api/report-plan` — deterministic report selection and analysis plan
- `GET /api/job-registry` — complete generated job registry
- `GET /api/job-registry/jobs/{id}` — one job registry entry
- `GET /api/docs` — interactive Scalar API reference
- `GET /api/openapi.json` or `/api/openapi.yaml` — machine-readable OpenAPI description
- `GET /api/schemas/{schema}.json` — generated JSON Schema for an API resource
