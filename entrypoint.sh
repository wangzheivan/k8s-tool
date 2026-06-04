#!/usr/bin/env bash
set -euo pipefail

MODE="${MODE:-agent}"

case "${MODE}" in
  agent)
    echo "starting k8s-tool-agent"
    exec /usr/local/bin/k8s-tool-server agent
    ;;
  server)
    echo "starting k8s-tool-server"
    exec /usr/local/bin/k8s-tool-server
    ;;
  *)
    echo "Unsupported MODE: ${MODE}. Use MODE=agent or MODE=server." >&2
    exit 1
    ;;
esac
