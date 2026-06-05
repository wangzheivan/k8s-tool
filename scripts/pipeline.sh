#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-harbor.rancherlsp.com/ivan/k8s-tool:v4.3}"
LOG_ROOT="${K8S_TOOL_PIPELINE_LOG_ROOT:-/tmp/k8s-tool-pipeline/logs}"
RUN_ID="${K8S_TOOL_PIPELINE_RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
LOG_DIR="${LOG_ROOT}/${RUN_ID}"
PUSH_GITHUB="false"
PUSH_HARBOR="false"
NO_PROMPT="false"
COMMIT_MESSAGE=""

mkdir -p "${LOG_DIR}"

usage() {
  cat <<'EOF'
Usage: scripts/pipeline.sh [options]

Options:
  --push-github          Commit local changes if needed, then push main to GitHub.
  --push-harbor          Push IMAGE_NAME to Harbor after build and smoke tests.
  --message <message>    Commit message used when --push-github creates a commit.
  --no-prompt            Do not ask interactive publish questions.
  -h, --help             Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --push-github)
      PUSH_GITHUB="true"
      shift
      ;;
    --push-harbor)
      PUSH_HARBOR="true"
      shift
      ;;
    --message)
      COMMIT_MESSAGE="${2:-}"
      shift 2
      ;;
    --no-prompt)
      NO_PROMPT="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

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
  if (cd "${REPO_ROOT}" && K8S_TOOL_PIPELINE_LOG_ROOT="${LOG_ROOT}" K8S_TOOL_PIPELINE_RUN_ID="${RUN_ID}" "$@") >"${log}" 2>&1; then
    printf '[PASS] %s - %s\n' "${name}" "${log}"
  else
    printf '[FAIL] %s - %s\n' "${name}" "${log}" >&2
    tail_log "${log}"
    exit 1
  fi
}

yes_answer() {
  local answer
  answer="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  [[ "${answer}" == "y" || "${answer}" == "yes" ]]
}

ask_publish_questions() {
  local answer
  if [[ "${PUSH_GITHUB}" != "true" ]]; then
    read -r -p "Push to GitHub main? [y/N] " answer
    if yes_answer "${answer}"; then
      PUSH_GITHUB="true"
    fi
  fi
  if [[ "${PUSH_HARBOR}" != "true" ]]; then
    read -r -p "Push to Harbor? [y/N] " answer
    if yes_answer "${answer}"; then
      PUSH_HARBOR="true"
    fi
  fi
}

ensure_main_branch() {
  local branch
  branch="$(git -C "${REPO_ROOT}" branch --show-current)"
  if [[ "${branch}" != "main" ]]; then
    echo "GitHub push requires main branch; current=${branch}" >&2
    exit 1
  fi
}

ensure_no_generated_artifacts() {
  rm -rf \
    "${REPO_ROOT}/frontend/node_modules" \
    "${REPO_ROOT}/frontend/dist" \
    "${REPO_ROOT}/frontend/tsconfig.tsbuildinfo"
}

push_github() {
  ensure_main_branch
  git -C "${REPO_ROOT}" remote set-url origin git@github.com:wangzheivan/k8s-tool.git
  ensure_no_generated_artifacts

  if [[ -n "$(git -C "${REPO_ROOT}" status --porcelain=v1)" ]]; then
    if [[ -z "${COMMIT_MESSAGE}" && "${NO_PROMPT}" != "true" ]]; then
      read -r -p "Commit message: " COMMIT_MESSAGE
    fi
    if [[ -z "${COMMIT_MESSAGE}" ]]; then
      echo "Commit message is required when local changes exist." >&2
      exit 1
    fi
    git -C "${REPO_ROOT}" add .
    git -C "${REPO_ROOT}" commit -m "${COMMIT_MESSAGE}"
  fi

  local local_sha
  local remote_sha
  local_sha="$(git -C "${REPO_ROOT}" rev-parse HEAD)"
  remote_sha="$(git -C "${REPO_ROOT}" ls-remote origin refs/heads/main | awk '{print $1}')"
  if [[ "${local_sha}" == "${remote_sha}" ]]; then
    printf '[OK] github already up to date - %s\n' "${local_sha}"
    return
  fi
  git -C "${REPO_ROOT}" push origin main
  remote_sha="$(git -C "${REPO_ROOT}" ls-remote origin refs/heads/main | awk '{print $1}')"
  if [[ "${local_sha}" != "${remote_sha}" ]]; then
    echo "GitHub push verification failed: local=${local_sha} remote=${remote_sha}" >&2
    exit 1
  fi
  printf '[OK] github pushed - %s\n' "${local_sha}"
}

push_harbor() {
  PUSH=true IMAGE_NAME="${IMAGE_NAME}" "${SCRIPT_DIR}/build-image.sh"
  printf '[OK] harbor pushed - %s\n' "${IMAGE_NAME}"
}

printf '[INFO] pipeline run id=%s image=%s logs=%s\n' "${RUN_ID}" "${IMAGE_NAME}" "${LOG_DIR}"
run_stage "validate" "${SCRIPT_DIR}/validate.sh"
run_stage "build-image" "${SCRIPT_DIR}/build-image.sh"
run_stage "smoke-test" "${SCRIPT_DIR}/smoke-test.sh" "${IMAGE_NAME}"

if [[ "${NO_PROMPT}" != "true" ]]; then
  ask_publish_questions
fi

if [[ "${PUSH_GITHUB}" == "true" ]]; then
  run_stage "push-github" push_github
else
  printf '[SKIP] push-github\n'
fi

if [[ "${PUSH_HARBOR}" == "true" ]]; then
  run_stage "push-harbor" push_harbor
else
  printf '[SKIP] push-harbor\n'
fi

ensure_no_generated_artifacts
printf '[OK] pipeline complete - logs=%s\n' "${LOG_DIR}"
