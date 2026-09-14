# HyperShift CI Health

A dashboard for the health of HyperShift's required presubmit jobs, release
payload blockers, and Component Readiness blockers. The backend reads job
definitions from a generated job registry and fetches policy and run data from
Sippy.

## Local development

### Prerequisites

- Go 1.25+
- A generated job registry in JSON format
- `oc` for deployment

```bash
go run . serve --dev --job-registry=/path/to/job-registry.json
```

`--dev` serves `index.html` from the filesystem for live editing. The server
only consumes the registry file; producing and distributing that file are
separate concerns.

## Job registry command

The standalone command generates a registry from an `openshift/release`
checkout:

```bash
go run . job-registry --release-dir=/path/to/openshift-release > job-registry.json
```

The reusable registry API lives in `jobregistry`. The dashboard's explicit
presubmit-to-periodic relationships live in `jobs/config.go`; startup validates
every referenced ID and obtains all job metadata from the registry.

## Deployment

On every pod start, an init container makes a shallow, blob-filtered sparse
clone of the job and release-controller configuration from `openshift/release`.
It generates `job-registry.json` in an `emptyDir` shared read-only with the
server container. The cluster therefore needs outbound access to GitHub.

```bash
make setup
make deploy
```

## API

- `GET /` — dashboard UI
- `GET /api/health` — job health snapshot for the 1-week, 2-week, and 1-month windows
- `GET /api/health/status` — current Sippy collection progress and errors
