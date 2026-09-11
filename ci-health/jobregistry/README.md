# HyperShift job registry

Package `jobregistry` defines, discovers, loads, and validates the HyperShift
job registry. The `ci-health job-registry` subcommand can serialize a registry
discovered from an `openshift/release` checkout.

## Command usage

```console
make build
./bin/ci-health job-registry > job-registry.json
```

Output is JSON.

`--release-dir` is required and selects the `openshift/release` checkout.

## Discovery scope

Discovery includes:

- all periodics and presubmits under
  `ci-operator/jobs/openshift/hypershift`;
- jobs elsewhere in `ci-operator/jobs` whose Prow context or CI target matches
  `hypershift-*-conformance`.

Discovery also reads release-controller configurations
under `core-services/release-controller/_releases`. Entries in each
configuration's `verify` map are joined to registry jobs by exact Prow job
name.

## Design principles and invariants

- A job is the unit of record. The registry does not encode report categories;
  consumers compose reports by filtering job properties.
- Each Prow job appears at most once and has a globally unique stable ID. Jobs
  are emitted in ID order so unchanged inputs produce stable output.
- Every job remains traceable to its generated definition in
  `openshift/release`.
- Release-controller participation is a job property rather than a report
  category. A job may have multiple stream relationships, and every
  relationship remains traceable to the exact `verify` declaration that
  created it.
- Release-controller roles are effective values: disabled takes precedence,
  followed by async, informing (`optional: true`), and blocking. Omitted
  release-controller booleans retain their documented false defaults.
- Multi-valued facts such as versions and platforms remain arrays. Unknown or
  inapplicable values are empty, null, or omitted rather than represented by
  sentinel values such as `main`.
- Type-specific properties are nested. Presubmit behavior does not appear on
  periodic jobs.
- Job discovery depends only on the selected `openshift/release` checkout. The
  package does not clone repositories or query live services.
- Sippy URLs are deterministic navigation hints and never assertions about
  current Sippy data availability.
- JSON is the serialization of the versioned registry API.

## Registry API

The versioned registry schema and field semantics are defined by the documented
Go types in [api.go](api.go). Import the package as
`github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry`; use
`Discover` to build a registry from a checkout or `Load`/`LoadFile` to consume
a generated registry.

## External links

Every job includes a deterministic OpenShift Prow job-history URL. Presubmits
and periodics use their respective Prow storage paths. Use `--prow-base-url` to
generate links for another Prow deployment.

Sippy URLs are generated deterministically without querying Sippy. Presubmits
link to the `Presubmits` analysis view, and release periodics link to their
release view. Periodics without a concrete release have `sippy_url: null`.

Release-controller relationships also contain links to the corresponding
Sippy stream overview and release payload status page. Stream metadata and the
Sippy route are derived from recognized OCP stream names such as
`5.1.0-0.ci`, `5.1.0-0.nightly`, and `5.1.0-0.nightly-multi`. Unrecognized
stream names retain their exact name and release status link but have a null
Sippy URL and empty derived fields.

A Sippy URL is a navigation hint, not evidence that Sippy has data for the job.
Sippy's public jobs API is time-windowed and cannot reliably distinguish an
unsupported job from one that has not run recently. Use `--sippy-base-url` to
generate links for another Sippy deployment.

Use `--sippy-stream-base-url` to generate release stream links for another
Sippy deployment and `--release-status-base-url` to generate payload status
links for another release-controller deployment. By default, amd64 links use
the central release status application while other recognized architectures
use their architecture-specific OCP release-controller host. An explicit
`--release-status-base-url` overrides that routing for every architecture.
These links are navigation hints and are not validated.
