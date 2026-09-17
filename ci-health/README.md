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

## UI development

The dashboard is a no-build Preact application using HTM templates and native
browser modules. Preact, HTM, and Fuse.js are pinned and embedded in the Go
binary; production does not load code from a CDN and does not require Node.js.

Generate the data artifacts once, then run the development server:

```bash
make dev
```

Development mode reads files under `assets/` directly and disables browser
caching, so editing a component or stylesheet only requires a page refresh.
`make test` runs the Go tests and, when Node.js is already available, syntax
checks the application modules. Node is optional and is not part of the build.

Each dashboard perspective has a shareable path: `/presubmits`, `/payload`,
`/component-readiness`, and `/registry`. View state is encoded in query
parameters (`window`, `group`, repeatable `platform`, `release-newest`, `release-oldest`,
and registry search `q`). The URL is the sole persisted source of UI state;
navigation works with browser history and the application does not use local
storage. The root path redirects to `/presubmits`.

Browser behavior is covered by an optional Playwright suite. Install its pinned
test dependency and Chromium once, then run it against the production-embedded
UI and the generated local artifacts:

```bash
make install-e2e
make test-e2e
```

Playwright and Node.js are development-only dependencies. Test traces are
retained under `/tmp/ci-health-playwright-results` when a browser test fails.

UI responsibilities are split between:

- `assets/web/app.js` — data loading, application state, and URL synchronization;
- `assets/web/ui.js` — pure display and grouping helpers;
- `assets/web/components/` — controls, health views, charts, and registry views; and
- `assets/web/styles.css` — the dashboard visual system.

The UI consumes the private per-window dashboard projection and the public job
registry API. It does not reach into report-plan or observation artifacts.

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
