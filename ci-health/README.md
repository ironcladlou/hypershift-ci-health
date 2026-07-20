# HyperShift Merge Queue Health

A single-page dashboard that surfaces the health of hypershift's merge-blocking presubmit jobs by consuming [Sippy's](https://sippy.dptools.openshift.org) API, with merge probability and retest estimates derived from empirical Prow data for recently merged PRs.

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

## What it shows

- **Merge summary** — first-try merge probability, median and P90 retests, and queue blocker count, all derived from empirical retest data for recently merged PRs via Prow
- **Blocking job table** — merge-blocking presubmit jobs sorted by pass rate
- **Presubmit-to-periodic pairing** — each presubmit paired with its periodic counterpart as a sub-row
- **Fail rate charts** — per-slot error rate line charts, presubmit in blue, periodic in orange
- **Sparkline bars** — per-slot pass/fail colored bars with correlation markers
- **Flake badges** — periodic jobs with flaky runs link to Sippy drill-downs
- **Alert banner** — tests newly failing across blocking jobs
- **Time window toggle** — 2d/7d switch with no network round-trip

## Design

**Single file, no build step.** The entire app is one `index.html` — HTML, CSS, JS, plus Chart.js from CDN. The HTML is embedded into the Go binary at build time.

**Stale-while-revalidate cache.** On page load, renders instantly from `localStorage` cached state, then fetches fresh data in the background. Auto-refreshes every 5 minutes.

**Retest analysis.** The server runs a background goroutine that periodically scrapes Prow pr-history pages for recently merged PRs to compute empirical merge probability and retest statistics. Results are served at `/api/retests` and consumed by the dashboard.

**Static config over dynamic discovery.** Which jobs block merges, which presubmit maps to which periodic, and which Sippy release each lives in — all hardcoded in `BLOCKING_JOBS`. These change rarely and are not discoverable from Sippy's API alone.

## Development tools

- `snapshot.sh` — captures a screenshot and rendered DOM dump via headless Chrome for visual debugging

## Reference projects

The sippy source is in `../upstreams/sippy/`.

The openshift CI source with job definitions is in `../upstreams/openshift-release/`.
