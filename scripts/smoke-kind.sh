#!/usr/bin/env bash
# Opt-in kind smoke test for the Kubernetes manifests. Creates a cluster, loads
# the locally-built images, applies the dev overlay, and waits for readiness.
#
# Usage: scripts/smoke-kind.sh [up|test|down]
# NOTE: run with Docker Compose STOPPED — a kind node plus the stack will not fit
# in ~2 GiB of RAM.
set -euo pipefail
cd "$(dirname "$0")/.."

CLUSTER="otel-genai-smoke"
cmd="${1:-up}"

case "$cmd" in
  up)
    kind create cluster --name "$CLUSTER" --wait 90s
    for img in collector api worker db-adapter llm-simulator; do
      echo "loading otel-genai/$img:latest ..."
      kind load docker-image "otel-genai/$img:latest" --name "$CLUSTER"
    done
    kubectl apply -k deploy/k8s/overlays/dev
    kubectl -n otel-genai rollout status deploy/collector --timeout=120s
    kubectl -n otel-genai rollout status deploy/api --timeout=120s
    echo
    echo "Smoke cluster is up. Try it:"
    echo "  kubectl -n otel-genai port-forward svc/api 8080:8080 &"
    echo "  curl localhost:8080/ | jq"
    echo "  kubectl -n otel-genai port-forward svc/collector 8889:8889 &"
    echo "  curl localhost:8889/metrics | grep gen_ai_"
    ;;
  test)
    kubectl -n otel-genai get pods -o wide
    kubectl -n otel-genai logs deploy/collector --tail=20
    ;;
  down)
    kind delete cluster --name "$CLUSTER"
    ;;
  *)
    echo "usage: scripts/smoke-kind.sh [up|test|down]" >&2
    exit 1
    ;;
esac
