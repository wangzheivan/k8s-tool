# Changelog

All notable changes to `k8s-tool` are documented here.

## v4.1 - Layered Network Diagnostics

- Added layered network matrix diagnostics:
  - Pod-to-Pod
  - Node-to-Node
  - Node-to-Pod
  - Pod-to-Node
- Added server APIs:
  - `GET /api/layered-network-check`
  - `POST /api/layered-network-check`
- Added agent API:
  - `POST /api/layered-network-check`
- Added host network namespace checks through `nsenter -t 1 -n`.
- Added `hostPID: true` to the agent DaemonSet.
- Added `util-linux` to the image for `nsenter`.
- Updated the React Network Diagnostics UI with layered tabs and aggregated failures.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v4.1`

## v4.0 - React Frontend Refactor

- Replaced server-side Go template UI with a React + Vite + TypeScript SPA.
- Kept Go backend APIs and changed the server to host frontend static assets.
- Added frontend components for agent status, network diagnostics, summaries, failures, and source aggregation.
- Added a Node builder stage to the Dockerfile.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v4.0`

## v3.1 - Network Diagnostics UI Aggregation

- Kept full Pod-to-Pod N x N network check results in the API.
- Changed the UI default view to Summary, By Source, and Failures instead of rendering all raw rows.
- Added source-level success, failure, skipped, ping, and HTTP aggregation.
- Added centralized failure details for easier review in larger clusters.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v3.1`

## v3.0 - Agent Status and Pod Network Diagnostics

- Expanded agent state handling beyond `online`.
- Added status values for non-running pods, missing Pod IPs, connection errors, HTTP errors, and invalid JSON.
- Renamed UI field `Hostname` to `Pod Name`.
- Changed memory display to GiB.
- Added full Pod-to-Pod network matrix checks using `ping` and HTTP checks against agent Pod IPs.
- Added more server and agent logs for troubleshooting.
- Split server HTML from Go logic into a template.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v3.0`

## v0.2.0 - Single Image Agent/Server Architecture

- Introduced single-image dual-mode startup:
  - `MODE=agent`
  - `MODE=server`
- Changed agent deployment to a DaemonSet.
- Added server Deployment for aggregated agent status.
- Added `GET /api/node-info` on the agent.
- Added Kubernetes API-based agent discovery in the server.
- Added agent Headless Service and server NodePort Service.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v0.2.0`

## Initial Linux/Kubernetes Tooling

- Moved maintenance workflow to a Linux host environment.
- Replaced Windows-oriented build flow with bash scripts.
- Set the default image registry to `harbor.rancherlsp.com/ivan/k8s-tool`.
- Added read-only Kubernetes RBAC with ServiceAccount, ClusterRole, and ClusterRoleBinding.
- Removed default admin kubeconfig mounting.
- Added common troubleshooting tools including `kubectl`, `curl`, `nslookup`, `netstat`, `jq`, and `ping`.
- Added an nginx-based port 80 troubleshooting page.
