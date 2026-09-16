# Golden job registry

`job-registry.json` is a checked-in integration fixture generated from
`openshift/release` commit `2c1442af1b8e87fd75c9fea43b5bc4b4ac0936b1`.
It intentionally does not change during a test run.

To refresh it after an intentional registry or source-data change, build the
binary and generate a replacement from the pinned checkout:

```console
make build
make update-golden RELEASE_DIR=/path/to/openshift-release
make test
```

Review the fixture diff and update the snapshot-specific table expectations in
the tests as part of the same change.
