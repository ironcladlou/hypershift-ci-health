# Sippy observations

`sippy` is the external-service adapter. Wire responses are converted into a
versioned `sippy-observation/v1` snapshot containing only Sippy-owned facts:
Component Readiness tier membership, release-scoped job analyses, recent
failures, and a result for every collection request.

Collection first discovers Component Readiness membership, then queries the
deduplicated union of those standard-tier jobs and the report plan's static
targets. Request failures make an observation partial but do not discard
successful evidence.

Request retry and terminal failure diagnostics print the full request URL,
including encoded query parameters, to stderr. Invalid JSON responses also
print their request URL so failing queries can be reproduced for debugging.
