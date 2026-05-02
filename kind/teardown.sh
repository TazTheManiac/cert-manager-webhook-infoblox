#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-cert-manager}"

if ! command -v helm &>/dev/null; then
  echo "ERROR: 'helm' is required but not found in PATH" >&2
  exit 1
fi

echo "==> Uninstalling cert-manager-webhook-infoblox..."
helm uninstall cert-manager-webhook-infoblox --namespace "${NAMESPACE}"
echo "Done."
