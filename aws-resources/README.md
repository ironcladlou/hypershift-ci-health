# HyperShift E2E Resource Monitor

Discovers AWS resources tagged by HyperShift e2e tests, correlates them with
Prow job status, and identifies orphaned resources left behind by completed or
GC'd jobs.

## How it works

1. **Collect** scans AWS regions for resources tagged with
   `hypershift.openshift.io/prow-job-id` using per-service EC2 Describe APIs
   (VPCs, subnets, route tables, gateways, endpoints, security groups,
   instances, EIPs, key pairs, etc.) and the Resource Groups Tagging API for
   IAM and Route53 resources. Checks each job's status via the Prow Deck API
   and writes the results to a JSON file.

2. **Serve** runs the dashboard:
   - By default, periodically collects data and serves the dashboard and
     `/api/data` endpoint from an in-memory store. This is how the app runs
     in-cluster.
   - `serve --data-file data.json` — serves data from a pre-collected JSON
     file for local development.

3. **Setup / Teardown** — provisions (or removes) the IAM role and cluster-specific
   Kustomize patches needed for deployment. See [Setup](#setup) below.

## Usage

```
# Collect data from all US regions (requires AWS credentials)
aws-resources collect

# Collect from specific regions
aws-resources collect --regions us-east-1,us-west-2

# Filter to a single prow job
aws-resources collect --job-id 03b1260e-2656-401c-b95d-43dfede165c8

# Write to a specific file
aws-resources collect --output snapshot.json

# Serve with live collection (default, used in-cluster)
aws-resources serve
aws-resources serve --interval 15m

# Serve from a pre-collected data file (local development)
aws-resources serve --data-file data.json
```

## Building

```
make build          # native binary
make build-linux    # static linux/amd64 binary (for container image)
make image          # cross-compile + oc start-build (requires deployed BuildConfig)
```

Requires Go 1.24+.

## In-cluster deployment

The app runs as a single container that collects data periodically and serves
the dashboard.

### Setup

The `setup` subcommand idempotently provisions everything needed for in-cluster
deployment. It writes gitignored Kustomize patches so no cluster-specific values
are committed to source control.

```
make setup
```

**IAM** — creates role `hypershift-ci-aws-resources` (path `/hypershift-ci/`)
with `tag:GetResources` permission and an OIDC trust policy for the
ServiceAccount. Tags the role for positive identification
(`managed-by: hypershift-ci-health`). Writes `deploy/sa-role-patch.yaml` with
the role ARN annotation.

**Route** — discovers the cluster's ingress domain and writes
`deploy/route-patch.yaml` with the route hostname.

Use `--oidc-provider-arn` to override OIDC auto-discovery, `--dry-run` to
preview.

To remove:

```
make teardown   # deletes IAM role and patch files
```

### Deploy

```
make deploy   # apply manifests, build image, rollout
```

This applies the Kustomize manifests (namespace, SA, BuildConfig, ImageStream,
Deployment, Service, Route), builds the container image via OpenShift binary
build, and restarts the deployment.

## Dashboard features

- Summary cards showing orphaned, running, and unknown job/resource counts
- Charts: resource state donut, orphan result breakdown, orphan age histogram,
  resources-per-job distribution, resource type breakdown
- **Jobs tab**: collapsible job tree with state badges, Prow links, age, and
  resource details grouped by type with AWS console links
- **Resources tab**: flat searchable table of all resources with type, region,
  and state filters
- Filtering by job state and resource type
