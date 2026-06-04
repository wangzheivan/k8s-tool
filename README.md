# K8s Troubleshooting Tool

`k8s-tool` 是一个部署到 Kubernetes 集群中的排障工具。当前架构使用单镜像双模式：

- `MODE=agent`：以 DaemonSet 运行在每个节点上，作为节点信息采集和网络诊断 agent。
- `MODE=server`：以 Deployment 运行，主动发现 agent，汇总状态并触发跨节点 Pod 网络诊断。

默认镜像地址：

```text
harbor.rancherlsp.com/ivan/k8s-tool:v3.1
```

## 功能

`k8s-tool-agent`：

- DaemonSet 部署，每个节点一个 Pod。
- 特权模式运行。
- 内置 `kubectl`、`curl`、`nslookup`、`netstat`、`jq`、`ping` 等常用命令。
- `GET /` 展示简洁页面：Pod Name、Pod IP、Node IP。
- `GET /api/node-info` 返回 Pod、Node、hostname、内存和采集时间。
- `POST /api/network-check` 对指定目标 Pod IP 执行 `ping` 和 HTTP 检查。

`k8s-tool-server`：

- Deployment 部署。
- 通过 Kubernetes API 发现同 namespace 下的 agent Pod。
- 每 10 秒后台刷新 agent 基础状态。
- UI 显示所有发现到的 agent，包括非 Running、无 Pod IP、连接失败、HTTP 错误和 JSON 错误。
- 手动触发 Network Diagnostics，执行全量 agent 到 agent 的 Pod 网络矩阵检查。
- Network Diagnostics 默认展示 Summary、By Source 聚合和 Failures 失败明细，完整 N×N 结果保留在 API 中。

## 构建和推送

```bash
./scripts/preflight.sh
./scripts/build-image.sh
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

Server:

- `GET /`
- `GET /api/agents`
- `POST /api/refresh`
- `GET /api/network-check`
- `POST /api/network-check`

## 使用其他 Namespace

当前清单默认部署到 `default` namespace。由于 `ClusterRoleBinding` 需要写明 ServiceAccount 所在 namespace，如果要部署到其他 namespace，需要同步修改 `k8s/k8s-tool.yaml` 中：

```yaml
subjects:
  - kind: ServiceAccount
    name: k8s-tool
    namespace: default
```

## 安全说明

agent 以特权模式运行，适合受控排障场景。server 使用 ServiceAccount 只读权限发现 agent Pod，不需要挂载 admin kubeconfig。Network Diagnostics 只在手动点击时执行，避免持续产生跨节点探测流量。
