#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

IMAGE_NAME="${IMAGE_NAME:-harbor.rancherlsp.com/ivan/k8s-tool:v4.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-stable}"
PLATFORM="${PLATFORM:-linux/amd64}"
PUSH="${PUSH:-false}"

PLATFORM="${PLATFORM}" "${SCRIPT_DIR}/preflight.sh"

if [[ "${PLATFORM}" == *,* && "${PUSH}" != "true" ]]; then
  echo "multi-arch builds must be pushed directly. Set PUSH=true." >&2
  exit 1
fi

if [[ "${PUSH}" == "true" ]]; then
  docker buildx build \
    --platform "${PLATFORM}" \
    --build-arg "KUBECTL_VERSION=${KUBECTL_VERSION}" \
    -t "${IMAGE_NAME}" \
    --push \
    "${REPO_ROOT}"
else
  docker build \
    --platform "${PLATFORM}" \
    --build-arg "KUBECTL_VERSION=${KUBECTL_VERSION}" \
    -t "${IMAGE_NAME}" \
    "${REPO_ROOT}"
fi
