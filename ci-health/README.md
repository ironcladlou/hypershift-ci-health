# HyperShift Merge Queue Health

A dashboard that surfaces the health of HyperShift's merge-blocking presubmit jobs, with merge probability and retest estimates derived from empirical Prow data for recently merged PRs.

The Go backend fetches data from [Sippy](https://sippy.dptools.openshift.org) and Prow periodically, transforms it, and holds it in memory. The frontend is a single `index.html` that consumes API endpoints served by the backend.

## Deployment

Deployed via Kustomize as a Go binary built in-cluster with a BuildConfig.

First-time setup discovers the cluster's ingress domain, writes a gitignored
route patch, and copies a GitHub PAT for the retest analyzer:

```bash
make setup TOKEN_FILE=/path/to/github-pat.txt
```

Then deploy (or redeploy after changes):

```bash
make deploy
```

Requires `oc` logged into the target cluster.

## Local development

```bash
GITHUB_TOKEN=$(gh auth token) go run . serve --dev
```

The `--dev` flag serves `index.html` from the filesystem for live editing.
Without it, the embedded copy baked into the binary is served.

## CLI

The binary also has a standalone `retests` subcommand for one-off analysis:

```bash
GITHUB_TOKEN=$(gh auth token) go run . retests --window 7 --output retests.json
```

This scrapes Prow pr-history pages for recently merged PRs and produces a JSON
report with per-PR retest counts and aggregate statistics.

## API endpoints

- `GET /` — serves the dashboard UI
- `GET /api/health` — job health snapshot with pass rates, sparklines, alerts, and platform/version metadata for both 2d and 7d windows; refreshed from Sippy every 15 minutes (configurable via `--sippy-interval`)
- `GET /api/retests` — retest analysis with per-PR retest counts and aggregate statistics; refreshed from Prow every 30 minutes (configurable via `--interval`)

## What it shows

- **Merge summary** — first-try merge probability, median and P90 retests, and queue blocker count, all derived from empirical retest data for recently merged PRs via Prow
- **Blocking job table** — merge-blocking presubmit jobs with presubmit-to-periodic pairing, grouped by platform (AWS, Azure, GKE, KubeVirt) and version (5.0, 4.22)
- **Fail rate charts** — per-slot error rate line charts for presubmit and periodic jobs
- **Sparkline bars** — per-slot pass/fail colored bars with correlation markers
- **Flake badges** — periodic jobs with flaky runs link to Sippy drill-downs
- **Alert banner** — tests newly failing across blocking jobs
- **Time window toggle** — 2d/7d switch with no network round-trip
- **Group-by toggle** — None, Platform, Version, or Platform / Version (default)

## Design

**Server-side data pipeline.** The Go backend fetches from Sippy's API periodically, transforms the data (sparkline bucketing, correlation analysis, infra/test fail classification, flake counting), and serves the result at `/api/health`. The frontend is a thin renderer with no direct Sippy calls.

**Job config in Go.** Which jobs block merges, which presubmit maps to which periodic, platform assignments, and Sippy release mappings are defined in `jobs/config.go`. The set of Sippy releases to fetch is derived from the job config automatically.

**Retest analysis.** A background goroutine scrapes Prow pr-history pages for recently merged PRs to compute empirical merge probability and retest statistics. Results are served at `/api/retests`.

## Development tools

- `snapshot.sh` — captures a screenshot and rendered DOM dump via headless Chrome for visual debugging

## Reference projects

The Sippy source is in `../upstreams/sippy/`.

The OpenShift CI source with job definitions is in `../upstreams/openshift-release/`.
