# Report plan

`reportplan` derives the deterministic, dashboard-specific selection from a
validated job registry and explicit selection policy. A plan contains stable
job references, release scope, relationships selected for the report, and the
static Sippy analysis query plan. It contains no live Sippy facts, registry
pointers, copied job definitions, or presentation labels.

Plans use the versioned `report-plan/v1` JSON contract and record the exact
registry digest from which they were built. Loading a plan validates the digest,
every reference, and that the artifact is the canonical result of its serialized
policy and registry.
