#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
LOG_ROOT="${K8S_TOOL_PIPELINE_LOG_ROOT:-/tmp/k8s-tool-pipeline/logs}"
RUN_ID="${K8S_TOOL_PIPELINE_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_DIR="${LOG_ROOT}/${RUN_ID}"

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

cleanup_frontend_artifacts() {
  rm -rf \
    "${REPO_ROOT}/frontend/node_modules" \
    "${REPO_ROOT}/frontend/dist" \
    "${REPO_ROOT}/frontend/tsconfig.tsbuildinfo"
}

trap cleanup_frontend_artifacts EXIT

run_stage "tool-go" bash -lc 'go version'
run_stage "tool-npm" bash -lc 'npm --version'
run_stage "tool-docker" bash -lc 'docker --version'
run_stage "shell-syntax" bash -lc 'bash -n entrypoint.sh scripts/*.sh check-rke2-cert.sh assets/logs-collector/rancher2_logs_collector.sh'
run_stage "yaml-parse" python3 -c "import pathlib,yaml; [list(yaml.safe_load_all(p.read_text())) for p in sorted(pathlib.Path('k8s').glob('*.yaml'))]; print('yaml-ok')"
run_stage "go-test" bash -lc 'go test ./...'
run_stage "frontend-build" bash -lc 'cd frontend && npm ci && npm run build'
cleanup_frontend_artifacts

printf '[OK] validate complete - logs=%s\n' "${LOG_DIR}"
