# K8s Troubleshooting Tool

`k8s-tool` 是一个部署到 Kubernetes 集群中的排障工具。当前架构使用单镜像双模式：

- `MODE=agent`：以 DaemonSet 运行在每个节点上，作为节点信息采集和网络诊断 agent。
- `MODE=server`：以 Deployment 运行，主动发现 agent，汇总状态并触发分层网络诊断。

默认镜像地址：

```text
harbor.rancherlsp.com/ivan/k8s-tool:v4.2
```

## 功能

`k8s-tool-agent`：

- DaemonSet 部署，每个节点一个 Pod。
- 特权模式运行，并启用 `hostPID`，用于通过 `nsenter` 进入宿主网络命名空间执行节点视角检查。
- 内置 `kubectl`、`curl`、`nslookup`、`netstat`、`jq`、`ping` 等常用命令。
- `GET /` 展示简洁页面：Pod Name、Pod IP、Node IP。
- `GET /api/node-info` 返回 Pod、Node、hostname、内存和采集时间。
- `POST /api/network-check` 对指定目标 Pod IP 执行 `ping` 和 HTTP 检查。
- `POST /api/layered-network-check` 执行指定层级的 Pod 或 Node 视角网络检查。
- `POST /api/etcd/status` 在 RKE2 etcd 节点上通过宿主 `crictl exec` 执行只读 etcd 状态检查。

`k8s-tool-server`：

- Deployment 部署。
- 通过 Kubernetes API 发现同 namespace 下的 agent Pod。
- 每 10 秒后台刷新 agent 基础状态。
- 使用 React + Vite + TypeScript SPA 展示 UI，Go 后端继续提供 API 和静态资源托管。
- UI 显示所有发现到的 agent，包括非 Running、无 Pod IP、连接失败、HTTP 错误和 JSON 错误。
- 手动触发 Network Diagnostics，执行四类矩阵：Pod-to-Pod、Node-to-Node、Node-to-Pod、Pod-to-Node。
- Pod 视角在 agent Pod 网络命名空间内执行；Node 视角使用 `nsenter -t 1 -n` 在宿主网络命名空间内执行。
- Network Diagnostics 使用分层 Tabs 展示 Summary、By Source 聚合和 Failures 失败明细，完整 N×N 结果保留在 API 中。
- Etcd Status 模块先通过 Node label `node-role.kubernetes.io/etcd` 识别 etcd 节点，再只调用对应节点上的 agent。
- Etcd Status 展示 member list、endpoint status、endpoint health、alarm list、version 和 raw output。
- tcpdump 抓包模块不在 v4.2 中实现，计划作为后续功能。

## 构建和推送

```bash
./scripts/preflight.sh
./scripts/build-image.sh
```

如果只验证前端：

```bash
cd frontend
npm ci
npm run build
```

推送到 Harbor：

```bash
docker login harbor.rancherlsp.com
PUSH=true ./scripts/build-image.sh
```

## 部署

主清单是 `k8s/k8s-tool.yaml`，包含：

- ServiceAccount
- ClusterRole / ClusterRoleBinding
- agent Headless Service
- server NodePort Service
- agent DaemonSet
- server Deployment

部署：

```bash
kubectl apply -f k8s/k8s-tool.yaml
```

访问 server：

```bash
kubectl get svc k8s-tool-server -o wide
```

```text
http://<任意节点IP>:<k8s-tool-server NodePort>
```

## API

Agent:

- `GET /`
- `GET /api/node-info`
- `POST /api/network-check`
- `POST /api/layered-network-check`
- `POST /api/etcd/status`

Server:

- `GET /`
- `GET /api/agents`
- `POST /api/refresh`
- `GET /api/network-check`
- `POST /api/network-check`
- `GET /api/layered-network-check`
- `POST /api/layered-network-check`
- `GET /api/etcd/status`
- `POST /api/etcd/status`

## 使用其他 Namespace

当前清单默认部署到 `default` namespace。由于 `ClusterRoleBinding` 需要写明 ServiceAccount 所在 namespace，如果要部署到其他 namespace，需要同步修改 `k8s/k8s-tool.yaml` 中：

```yaml
subjects:
  - kind: ServiceAccount
    name: k8s-tool
    namespace: default
```

## 安全说明

agent 以特权模式运行并启用 `hostPID`，适合受控排障场景。v4.2 为 RKE2 etcd 检查额外挂载 `/var/lib/rancher/rke2` 和 `/run/k3s/containerd/containerd.sock`，仅用于在 etcd 节点执行只读 `etcdctl` 查询。server 使用 ServiceAccount 只读权限发现 agent Pod 和 Node，不需要挂载 admin kubeconfig。Network Diagnostics 和 Etcd Status 只在手动点击时执行，避免持续产生探测流量。
