#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

oc apply -f deploy/

oc -n ci-health create configmap ci-health-html \
  --from-file=index.html=index.html \
  --dry-run=client -o yaml | oc apply -f -

oc -n ci-health rollout restart deployment/ci-health
oc -n ci-health rollout status deployment/ci-health --timeout=60s
