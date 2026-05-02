#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

NAMESPACE="${NAMESPACE:-cert-manager}"
GROUP_NAME="${GROUP_NAME:-acme.example.com}"

if [[ -z "${KIND_CLUSTER:-}" ]]; then
  echo "ERROR: KIND_CLUSTER must be set to the name of your kind cluster" >&2
  exit 1
fi
IMAGE_NAME="tazthemaniac/cert-manager-webhook-infoblox"
IMAGE_TAG="$(grep '^appVersion:' "${ROOT_DIR}/charts/cert-manager-webhook-infoblox/Chart.yaml" | awk '{print $2}' | tr -d '"')"

# ── Prerequisites check ────────────────────────────────────────────────────────
for cmd in kind kubectl helm docker make; do
  if ! command -v "${cmd}" &>/dev/null; then
    echo "ERROR: '${cmd}' is required but not found in PATH" >&2
    exit 1
  fi
done

# ── Build and load image ───────────────────────────────────────────────────────
echo "==> Building Docker image ${IMAGE_NAME}:${IMAGE_TAG}..."
make -C "${ROOT_DIR}" docker-build

echo "==> Loading image into kind cluster '${KIND_CLUSTER}'..."
kind load docker-image "${IMAGE_NAME}:${IMAGE_TAG}" --name "${KIND_CLUSTER}"

# ── Install webhook chart ──────────────────────────────────────────────────────
echo "==> Installing cert-manager-webhook-infoblox (groupName=${GROUP_NAME})..."
helm install cert-manager-webhook-infoblox "${ROOT_DIR}/charts/cert-manager-webhook-infoblox" \
  --namespace "${NAMESPACE}" \
  --set groupName="${GROUP_NAME}" \
  --set image.pullPolicy=Never \
  --wait

# ── Done ───────────────────────────────────────────────────────────────────────
echo ""
echo "Webhook pod:"
kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=cert-manager-webhook-infoblox
echo ""
echo "Next steps:"
echo "  1. Apply a ClusterIssuer as described in the root README."
echo ""
echo "  See kind/README.md for full details."
