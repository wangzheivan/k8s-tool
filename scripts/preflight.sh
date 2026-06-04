#!/usr/bin/env bash
set -euo pipefail

PLATFORM="${PLATFORM:-linux/amd64}"

check() {
  local name="$1"
  local detail="$2"
  printf '[OK] %s - %s\n' "${name}" "${detail}"
}

fail() {
  local name="$1"
  local detail="$2"
  printf '[FAIL] %s - %s\n' "${name}" "${detail}" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "Docker CLI" "docker was not found"
check "Docker CLI" "$(command -v docker)"

docker info >/dev/null 2>&1 || fail "Docker daemon" "docker is not running or current user cannot access it"
check "Docker daemon" "$(docker version --format 'Client {{.Client.Version}} / Server {{.Server.Version}}')"

case "${PLATFORM}" in
  linux/amd64|linux/arm64|linux/amd64,linux/arm64|linux/arm64,linux/amd64)
    check "Platform" "${PLATFORM}"
    ;;
  *)
    fail "Platform" "supported values: linux/amd64, linux/arm64, or linux/amd64,linux/arm64"
    ;;
esac

if [[ "${PLATFORM}" == *,* ]]; then
  docker buildx version >/dev/null 2>&1 || fail "Buildx" "multi-arch builds require docker buildx"
  check "Buildx" "$(docker buildx version)"
fi
