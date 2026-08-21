# HyperShift Merge Queue Health

A dashboard for HyperShift's merge-blocking presubmit jobs, with merge probability and retest estimates derived from Prow data.

The Go backend fetches from [Sippy](https://sippy.dptools.openshift.org) and Prow periodically and serves the results. The frontend is a single `index.html`.

## Local development

### Prerequisites

- Go 1.25+
- `oc` (for deployment)
- GitHub personal access token

```bash
GITHUB_TOKEN=$(gh auth token) go run . serve --dev
```

`--dev` serves `index.html` from the filesystem for live editing.

```bash
GITHUB_TOKEN=$(gh auth token) go run . retests --window 7 --output retests.json
```

One-off retest analysis for recently merged PRs.

## Deployment

Deployed via Kustomize with a BuildConfig. Requires `oc` logged into the target cluster.

```bash
make setup TOKEN_FILE=/path/to/github-pat.txt
make deploy
```

## Job configuration

Jobs are declared in `jobs/config.go` as a list of `JobSpec` entries. Set `FutureRelease` (e.g. `5.1`) and N-1 variant names and periodic mappings are derived automatically. To bump releases, change `FutureRelease`.

## API

- `GET /` — dashboard UI
- `GET /api/health` — job health snapshot (pass rates, sparklines, alerts) for 2d and 7d windows
- `GET /api/retests` — retest analysis with per-PR counts and aggregate statistics
