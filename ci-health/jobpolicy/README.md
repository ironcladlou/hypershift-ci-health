# Team job policy

Team policy separates reusable registry facts from a team's priorities and
health expectations. It produces a catalog covering every registry job without
collecting data or evaluating health. This lets teams refine their assumptions
without changing the existing dashboard's collection set.

Ranking uses ordered groups and dimensions, with a stable job ID to break ties.
The same registry, policy, and algorithm produce the same ordering; rank
positions can change as those inputs change. The
[HyperShift policy](policies/hypershift.yaml) defines the assumptions, so they
remain explicit and reviewable. Per-job exceptions require a rationale.

Structured health objectives tell downstream evaluators which checks to perform,
what evidence is sufficient, and which jobs form comparison cohorts. Declaring
an objective does not imply that it is satisfied. The
[assessment contracts](health_objective.go) define how evaluators should
interpret these objectives, including insufficient evidence.

Each catalog records its resolved policy, registry digest, and algorithm
version so decisions can be traced and reproduced. Loading verifies the catalog
against those inputs, preventing stale or altered rankings from being served.

The standalone `/policy` view makes rankings, objectives, and explanations
available for review, reusing the registry browser's controls and detail widgets.
External clients receive the same catalog through the public API; see the running
server's [API documentation](/api/docs).
