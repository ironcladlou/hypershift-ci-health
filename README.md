# HyperShift Merge Queue Health

A single-page dashboard that surfaces the health of hypershift's merge-blocking presubmit jobs by consuming [Sippy's](https://sippy.dptools.openshift.org) API.

## Problem

The hypershift repo has ~15 presubmit jobs that block merges to main. Today there's no single view that shows:

- All blocking jobs and their current pass rates
- Each presubmit correlated with its periodic counterpart (which may be in a different Sippy release)
- Acute signals: sudden pass rate drops, newly failing tests

Sippy has all the underlying data, but you have to navigate between the Presubmits, 5.0, and 4.22 releases manually and know which jobs to look for.

## What this adds over Sippy

1. **Blocking job config** — static mapping of which jobs gate merges (derived from Prow config)
2. **Presubmit-to-periodic mapping** — static config correlating each presubmit with its periodic counterpart across Sippy releases
3. **A single curated view** — one table with pass rates, trends, sparklines, and an alert banner for newly failing tests
4. **Time window selector** — 2d/7d toggle that aligns pass rates, sparklines, and periodic rates to the same window

## Running

```bash
python3 -m http.server 8080
open http://localhost:8080
```

## Design principles

**Single file, no build step.** The entire app is one `index.html` — HTML, CSS, JS. No framework, no bundler, no backend. Serve it with any static file server.

**Fetch max, narrow client-side.** All data for the widest time window (7d default + 2d via Sippy's `period=twoDay`) is fetched in one batch of parallel API calls. Switching the time window is a pure client-side re-read of pre-computed state — no network round-trip.

**Raw cache, computed state, clean boundary.** Raw Sippy API responses are cached in `localStorage` alongside pre-computed dashboard state for each time window. A single `localStorage.setItem` replaces both atomically on refresh. The UI only reads pre-computed state; rendering functions never touch raw API data.

**Stale-while-revalidate.** On page load, the dashboard renders instantly from cached state, then fetches fresh data in the background. A generation counter prevents stale in-flight fetches from overwriting after a window switch.

**Static config over dynamic discovery.** Which jobs block merges, which presubmit maps to which periodic, and which Sippy release each lives in — all hardcoded. These change rarely and are not discoverable from Sippy's API alone.

## Architecture

```
Sippy API ──fetch──▶ raw responses ──transform──▶ per-window state
                          │                            │
                          └──── localStorage ──────────┘
                                (atomic write)
                                      │
                          UI reads pre-computed state
```

- `fetchAllFreshData()` — 8 parallel API calls: 3 releases × 2 windows + recent failures + job runs
- `transformRawData(raw, windowKey)` — extracts job stats, correlates periodics, buckets sparklines, filters alerts
- `computeAllStates(raw)` — runs the transform for each window
- `saveToCache(raw, states)` — one atomic `localStorage` write
- `renderDashboard(state)` — reads only the pre-computed state for the active window

## Development tools

- `sippy_explore.py` — CLI utility for exploring Sippy API endpoints. Run with no args for usage.
- `screenshot.sh` — Captures a screenshot and HTML dump via headless Chrome for visual debugging.

## Presubmit-to-periodic mapping

| Presubmit | Periodic | Release |
|-----------|----------|---------|
| e2e-aws | e2e-aws-ovn | 5.0 |
| e2e-aks | e2e-aks | 5.0 |
| e2e-azure-v2-self-managed | e2e-azure-v2-self-managed | 5.0 |
| e2e-aws-upgrade-hypershift-operator | e2e-aws-upgrade | 5.0 |
| e2e-v2-gke | e2e-v2-gke | 5.0 |
| e2e-aws-4-22 | e2e-aws-ovn | 4.22 |
| e2e-aks-4-22 | e2e-aks | 4.22 |
| e2e-kubevirt-aws-ovn-reduced | e2e-kubevirt-aws-ovn-csi (approximate) | 4.22 |
| e2e-v2-aws | none | — |
| e2e-aws-override | none (trigger-scoped) | — |
| e2e-aks-override | none (trigger-scoped) | — |

## Sippy API endpoints used

| Endpoint | Purpose |
|----------|---------|
| `/api/jobs?release=Presubmits` | Presubmit pass rates (7d default, 2d via `period=twoDay`) |
| `/api/jobs?release={5.0,4.22}` | Periodic counterpart pass rates |
| `/api/jobs/runs?release=Presubmits` | Individual job runs for sparkline bucketing (1500 most recent) |
| `/api/tests/recent_failures?release=Presubmits&period=4h&previousPeriod=24h` | Newly failing tests for the alert banner |

## Related projects

| Path | What |
|------|------|
| `~/Projects/sippy` | Sippy source — API handlers in `pkg/api/`, routes in `pkg/sippyserver/server.go` |
| `~/Projects/openshift-release/ci-operator/jobs/openshift/hypershift/` | Prow job configs (which jobs block merges) |
| `~/Projects/openshift-release/ci-operator/config/openshift/hypershift/` | CI operator test definitions (presubmit and periodic configs) |
