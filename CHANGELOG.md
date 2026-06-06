# Changelog

All notable changes to `k8s-tool` are documented here.

## v4.4 - Logs Collector

- Added the Logs Collector module for full-cluster log bundle collection and download.
- Added server APIs `GET /api/logs/status`, `POST /api/logs/collect`, and `GET /api/logs/download/{collectionID}`.
- Added agent APIs `POST /api/logs/collect` and `GET /api/logs/download/{artifactID}`.
- Embedded the Rancher v2.x logs collector script as an image asset instead of downloading it at runtime.
- Added React UI controls for required log time range selection, collection progress, per-node status, and cluster archive download.
- Added read-only RBAC for pod logs and common workload resources used by collection.
- Added log collection environment variables: `LOG_COLLECTION_TIMEOUT_SECONDS`, `LOG_COLLECTION_MAX_PARALLEL`, and `LOG_COLLECTION_RETENTION_HOURS`.
- Added read-only agent hostPath mounts for `/var/log` and `/etc/rancher`.
- Updated image references to `harbor.rancherlsp.com/ivan/k8s-tool:v4.4`.

## v4.3 - RKE2 Certificate Status

- Added the RKE2 Certificate Status module for server and agent node certificate checks.
- Added agent API `POST /api/certs/status`.
- Added server APIs `GET /api/certs/status` and `POST /api/certs/status`.
- Added React UI summary, node aggregation, filters, and certificate detail views.
- Added certificate thresholds through `CERT_EXPIRING_DAYS` and `CERT_CHECK_TIMEOUT_SECONDS`.
- Updated image references to `harbor.rancherlsp.com/ivan/k8s-tool:v4.3`.
- tcpdump packet capture remains deferred to a later release.

## v4.2.1 - Etcd crictl Exec Network Namespace Fix

- Fixed Etcd Status checks failing with `dial tcp 127.0.0.1:10010: connect: connection refused`.
- Changed agent-side RKE2 `crictl ps` and `crictl exec` calls to run through `nsenter -t 1 -n`.
- Kept the agent DaemonSet off `hostNetwork` to avoid host port 80 conflicts.
- Kept the v4.2 Etcd Status API and UI behavior unchanged.
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v4.2.1`

## v4.2 - RKE2 Etcd Status Checks

- Added the Etcd Status module for RKE2 embedded etcd clusters.
- Added server-side etcd node discovery through the `node-role.kubernetes.io/etcd` Node label.
- Added server APIs:
  - `GET /api/etcd/status`
  - `POST /api/etcd/status`
- Added agent API:
  - `POST /api/etcd/status`
- Added agent-side read-only etcd checks through host RKE2 `crictl exec`:
  - `member list`
  - `endpoint status`
  - `endpoint health`
  - `alarm list`
  - `version`
- Added React UI for Etcd Status summary, per-node status, alarms, command results, and raw output.
- Added RKE2 hostPath mounts for `/var/lib/rancher/rke2` and `/run/k3s/containerd/containerd.sock`.
- Added etcd/control-plane tolerations so the agent can run on tainted RKE2 server nodes.
- Added environment variables:
  - `ETCD_NODE_SELECTOR`
  - `ETCD_CHECK_TIMEOUT_SECONDS`
  - `ETCD_COMMAND_TIMEOUT_SECONDS`
- Published image: `harbor.rancherlsp.com/ivan/k8s-tool:v4.2`

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
