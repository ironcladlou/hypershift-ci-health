# HyperShift Merge Queue Health

A single-page dashboard that surfaces the health of hypershift's merge-blocking presubmit jobs by consuming [Sippy's](https://sippy.dptools.openshift.org) API.

## Deployment

Deployed via Kustomize as nginx + a ConfigMap.

First-time setup discovers the cluster's ingress domain and writes a gitignored
route patch:

```bash
make setup
```

Then deploy (or redeploy after editing `index.html`):

```bash
make deploy
```

Requires `oc` logged into the target cluster.

## Local development

```bash
go run . &
open http://localhost:8080
```

## What it shows

- **Blocking job table** — merge-blocking presubmit jobs sorted by pass rate
- **Presubmit-to-periodic pairing** — each presubmit paired with its periodic counterpart as a sub-row
- **Fail rate charts** — per-slot error rate line charts, presubmit in blue, periodic in orange
- **Sparkline bars** — per-slot pass/fail colored bars with correlation markers
- **Flake badges** — periodic jobs with flaky runs link to Sippy drill-downs
- **Alert banner** — tests newly failing across blocking jobs
- **Time window toggle** — 2d/7d switch with no network round-trip

## Design

**Single file, no build step.** The entire app is one `index.html` — HTML, CSS, JS, plus Chart.js from CDN.

**Stale-while-revalidate cache.** On page load, renders instantly from `localStorage` cached state, then fetches fresh data in the background. Auto-refreshes every 5 minutes.

**Static config over dynamic discovery.** Which jobs block merges, which presubmit maps to which periodic, and which Sippy release each lives in — all hardcoded in `BLOCKING_JOBS`. These change rarely and are not discoverable from Sippy's API alone.

## Development tools

- `snapshot.sh` — captures a screenshot and rendered DOM dump via headless Chrome for visual debugging

## Reference projects

The sippy source is in `../upstreams/sippy/`.

The openshift CI source with job definitions is in `../upstreams/openshift-release/`.
