# HyperShift Merge Queue Health

A single-page dashboard that surfaces the health of hypershift's merge-blocking presubmit jobs by consuming [Sippy's](https://sippy.dptools.openshift.org) API.

## Running

```bash
python3 -m http.server 8080
open http://localhost:8080
```

## What it shows

- **Blocking job table** — all 11 e2e presubmit jobs that gate merges, sorted by pass rate
- **Presubmit-to-periodic pairing** — each presubmit is paired with its periodic counterpart (across Sippy releases 5.0 and 4.22) as a sub-row
- **Fail rate charts** — per-slot error rate line chart for each job (Chart.js), presubmit in blue, periodic in orange
- **Sparkline bars** — per-slot pass/fail colored bars with correlation markers (orange ticks above presubmit sparklines when the periodic also fails in that slot)
- **Flake badges** — periodic jobs with flaky runs link to Sippy drill-downs
- **Alert banner** — tests newly failing in the last 4 hours across blocking jobs
- **Time window toggle** — 2d/7d switch; re-renders from pre-computed state with no network round-trip

## Design

**Single file, no build step.** The entire app is one `index.html` — HTML, CSS, JS, plus Chart.js from CDN. No framework, no bundler, no backend.

**Stale-while-revalidate cache.** On page load, renders instantly from `localStorage` cached pre-computed state, then fetches fresh data in the background. Only pre-computed states are cached (not raw API data) to stay within `localStorage` size limits. Auto-refreshes every 5 minutes.

**Static config over dynamic discovery.** Which jobs block merges, which presubmit maps to which periodic, and which Sippy release each lives in — all hardcoded in `BLOCKING_JOBS`. These change rarely and are not discoverable from Sippy's API alone.

## Architecture

```
Sippy API ──fetch──▶ raw responses ──transform──▶ per-window state
                                                       │
                                               localStorage cache
                                               (pre-computed only)
                                                       │
                                              UI reads cached state
```

- `fetchAllFreshData()` — 10 parallel API calls: 3 releases × 2 windows + recent failures + 3 job-run queries
- `transformRawData(raw, windowKey)` — extracts job stats, correlates periodics, buckets sparklines, computes correlation, counts flakes, filters alerts
- `computeAllStates(raw)` — runs the transform for each window
- `saveToCache(states)` — one atomic `localStorage` write (pre-computed states only)
- `renderDashboard(state)` — reads pre-computed state for the active window, builds table HTML, initializes Chart.js charts

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
| e2e-kubevirt-aws-ovn-reduced | e2e-kubevirt-aws-ovn-csi | 4.22 |
| e2e-v2-aws | none | — |
| e2e-aws-override | none | — |
| e2e-aks-override | none | — |

## Sippy API endpoints used

| Endpoint | Purpose |
|----------|---------|
| `/api/jobs?release=Presubmits` | Presubmit pass rates (7d default, 2d via `period=twoDay`) |
| `/api/jobs?release={5.0,4.22}` | Periodic counterpart pass rates |
| `/api/jobs/runs?release=Presubmits` | Presubmit job runs for sparkline bucketing (1500 most recent) |
| `/api/jobs/runs?release={5.0,4.22}` | Periodic job runs for sparklines, flake counting, and correlation analysis |
| `/api/tests/recent_failures?release=Presubmits` | Newly failing tests for the alert banner |

## Development tools

- `screenshot.sh` — captures a screenshot and HTML dump via headless Chrome for visual debugging
