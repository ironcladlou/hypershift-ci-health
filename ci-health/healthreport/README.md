# Health reports

`healthreport` performs the pure evaluation step. It joins a registry, report
plan, and Sippy observation into the versioned `health-report/v2` projection
consumed by the UI. Evaluation performs no filesystem or network I/O and does
not mutate its inputs.

The report records registry and plan digests, observation timestamps,
completeness, and per-request collection results. Registry metadata enriches
Sippy-authoritative Component Readiness membership when a matching job exists;
an unmatched Sippy member remains in the report with `registry_missing: true`.

Each window owns one shared ordered timestamp list. Job sparklines align to that
list and encode slots as `[total, passes, testFailures, infraFailures]` tuples.
The serving API returns one window at a time.
