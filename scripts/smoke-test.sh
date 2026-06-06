#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
IMAGE_NAME="${1:-${IMAGE_NAME:-harbor.rancherlsp.com/ivan/k8s-tool:v4.4}}"
LOG_ROOT="${K8S_TOOL_PIPELINE_LOG_ROOT:-/tmp/k8s-tool-pipeline/logs}"
RUN_ID="${K8S_TOOL_PIPELINE_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_DIR="${LOG_ROOT}/${RUN_ID}"
AGENT_NAME="${K8S_TOOL_AGENT_TEST_NAME:-k8s-tool-agent-test}"
SERVER_NAME="${K8S_TOOL_SERVER_TEST_NAME:-k8s-tool-server-test}"
AGENT_PORT="${K8S_TOOL_AGENT_TEST_PORT:-18080}"
SERVER_PORT="${K8S_TOOL_SERVER_TEST_PORT:-18081}"

mkdir -p "${LOG_DIR}"

log_path() {
  local name="$1"
  printf '%s/%s.log' "${LOG_DIR}" "${name}"
}

tail_log() {
  local log="$1"
  if [[ -f "${log}" ]]; then
    tail -40 "${log}" >&2 || true
  fi
}

run_stage() {
  local name="$1"
  shift
  local log
  log="$(log_path "${name}")"
  printf '[RUN] %s\n' "${name}"
  if (cd "${REPO_ROOT}" && "$@") >"${log}" 2>&1; then
    printf '[PASS] %s - %s\n' "${name}" "${log}"
  else
    printf '[FAIL] %s - %s\n' "${name}" "${log}" >&2
    tail_log "${log}"
    exit 1
  fi
}

cleanup() {
  docker rm -f "${AGENT_NAME}" "${SERVER_NAME}" >/dev/null 2>&1 || true
}

wait_http() {
  local url="$1"
  for _ in $(seq 1 30); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  curl -fsS "${url}" >/dev/null
}

trap cleanup EXIT
cleanup

run_stage "smoke-agent-start" docker run -d --name "${AGENT_NAME}" -e MODE=agent -p "127.0.0.1:${AGENT_PORT}:80" "${IMAGE_NAME}"
run_stage "smoke-agent-ready" wait_http "http://127.0.0.1:${AGENT_PORT}/api/node-info"
run_stage "smoke-agent-node-info" curl -fsS "http://127.0.0.1:${AGENT_PORT}/api/node-info"
run_stage "smoke-agent-certs" curl -fsS -X POST "http://127.0.0.1:${AGENT_PORT}/api/certs/status"
run_stage "smoke-agent-logs" curl -fsS -X POST -H "Content-Type: application/json" -d '{"days":1}' "http://127.0.0.1:${AGENT_PORT}/api/logs/collect"

run_stage "smoke-server-start" docker run -d --name "${SERVER_NAME}" -e MODE=server -p "127.0.0.1:${SERVER_PORT}:80" "${IMAGE_NAME}"
run_stage "smoke-server-ready" wait_http "http://127.0.0.1:${SERVER_PORT}/"
run_stage "smoke-server-root" curl -fsS "http://127.0.0.1:${SERVER_PORT}/"
run_stage "smoke-server-agents" curl -fsS "http://127.0.0.1:${SERVER_PORT}/api/agents"
run_stage "smoke-server-certs" curl -fsS "http://127.0.0.1:${SERVER_PORT}/api/certs/status"
run_stage "smoke-server-logs-status" curl -fsS "http://127.0.0.1:${SERVER_PORT}/api/logs/status"
run_stage "smoke-server-logs-bad-download" bash -lc "status=\$(curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:${SERVER_PORT}/api/logs/download/bad-id || true); test \"\${status}\" = 404"

cleanup
trap - EXIT

printf '[OK] smoke complete - image=%s logs=%s\n' "${IMAGE_NAME}" "${LOG_DIR}"
