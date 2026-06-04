#!/usr/bin/env bash
set -euo pipefail

MODE="${MODE:-agent}"
REFRESH_INTERVAL_SECONDS="${REFRESH_INTERVAL_SECONDS:-10}"

HTML_PATH="/usr/share/nginx/html/index.html"
API_DIR="/usr/share/nginx/html/api"
NODE_INFO_PATH="${API_DIR}/node-info"
TMP_HTML_PATH="/tmp/index.html"
TMP_NODE_INFO_PATH="/tmp/node-info.json"

mem_value_kb() {
  local key="$1"
  awk -v key="${key}" '$1 == key ":" { print $2; found=1 } END { if (!found) print 0 }' /proc/meminfo
}

write_agent_files() {
  local now pod_name namespace pod_ip node_name node_ip hostname
  now="$(date -Iseconds)"
  pod_name="${POD_NAME:-$(hostname)}"
  namespace="${POD_NAMESPACE:-default}"
  pod_ip="${POD_IP:-unknown}"
  node_name="${NODE_NAME:-unknown}"
  node_ip="${HOST_IP:-unknown}"
  hostname="$(hostname)"

  local mem_total mem_available mem_free mem_used
  mem_total="$(mem_value_kb MemTotal)"
  mem_available="$(mem_value_kb MemAvailable)"
  mem_free="${mem_available}"
  if [ "${mem_total}" -gt 0 ] && [ "${mem_available}" -ge 0 ]; then
    mem_used="$((mem_total - mem_available))"
  else
    mem_used="0"
  fi

  jq -n \
    --arg podName "${pod_name}" \
    --arg namespace "${namespace}" \
    --arg podIP "${pod_ip}" \
    --arg nodeName "${node_name}" \
    --arg nodeIP "${node_ip}" \
    --arg hostname "${hostname}" \
    --arg collectedAt "${now}" \
    --argjson memoryTotalKB "${mem_total}" \
    --argjson memoryFreeKB "${mem_free}" \
    --argjson memoryUsedKB "${mem_used}" \
    '{
      podName: $podName,
      namespace: $namespace,
      podIP: $podIP,
      nodeName: $nodeName,
      nodeIP: $nodeIP,
      hostname: $hostname,
      memoryTotalKB: $memoryTotalKB,
      memoryFreeKB: $memoryFreeKB,
      memoryUsedKB: $memoryUsedKB,
      collectedAt: $collectedAt
    }' > "${TMP_NODE_INFO_PATH}"
  mv "${TMP_NODE_INFO_PATH}" "${NODE_INFO_PATH}"

  cat > "${TMP_HTML_PATH}" <<EOF
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="${REFRESH_INTERVAL_SECONDS}">
  <title>k8s-tool-agent</title>
  <style>
    :root {
      color-scheme: light dark;
      font-family: Arial, Helvetica, sans-serif;
      background: #f6f8fb;
      color: #17202a;
    }
    body {
      margin: 0;
      padding: 24px;
    }
    main {
      max-width: 760px;
    }
    h1 {
      margin: 0 0 16px;
      font-size: 26px;
      letter-spacing: 0;
    }
    dl {
      display: grid;
      grid-template-columns: 150px 1fr;
      gap: 10px 14px;
      background: #ffffff;
      border: 1px solid #d9e2ec;
      border-radius: 8px;
      padding: 16px;
    }
    dt {
      color: #52606d;
      font-weight: 700;
    }
    dd {
      margin: 0;
      word-break: break-word;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        background: #121820;
        color: #eef2f6;
      }
      dl {
        background: #1c2633;
        border-color: #34445a;
      }
      dt {
        color: #b7c4d3;
      }
    }
  </style>
</head>
<body>
  <main>
    <h1>k8s-tool-agent</h1>
    <dl>
      <dt>Hostname</dt><dd>${hostname}</dd>
      <dt>Pod IP</dt><dd>${pod_ip}</dd>
      <dt>Node IP</dt><dd>${node_ip}</dd>
      <dt>Collected</dt><dd>${now}</dd>
    </dl>
  </main>
</body>
</html>
EOF
  mv "${TMP_HTML_PATH}" "${HTML_PATH}"
}

agent_loop() {
  mkdir -p "${API_DIR}" /run/nginx /usr/share/nginx/html
  write_agent_files || true
  while true; do
    sleep "${REFRESH_INTERVAL_SECONDS}"
    write_agent_files || true
  done
}

case "${MODE}" in
  agent)
    agent_loop &
    exec nginx -g "daemon off;"
    ;;
  server)
    exec /usr/local/bin/k8s-tool-server
    ;;
  *)
    echo "Unsupported MODE: ${MODE}. Use MODE=agent or MODE=server." >&2
    exit 1
    ;;
esac
