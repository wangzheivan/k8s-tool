# K8s Troubleshooting Tool

`k8s-tool` 是一个部署到 Kubernetes 集群中的排障工具。当前架构使用单镜像双模式：

- `MODE=agent`：以 DaemonSet 运行在每个节点上，作为节点信息采集 agent，并保留常用排障工具。
- `MODE=server`：以 Deployment 运行，主动发现并拉取 agent 信息，通过 Web UI 汇总展示。

默认镜像地址：

```text
harbor.rancherlsp.com/ivan/k8s-tool:v0.2.0
```

## 功能

`k8s-tool-agent`：

- DaemonSet 部署，每个节点一个 Pod。
- 特权模式运行。
- 内置 `kubectl`、`curl`、`nslookup`、`netstat`、`jq`、`ping` 等常用命令。
- 暴露 80 端口。
- `GET /` 展示简洁页面：hostname、Pod IP、Node IP。
- `GET /api/node-info` 返回 JSON：Pod、Node、hostname、内存和采集时间。

`k8s-tool-server`：

- Deployment 部署。
- 通过 Kubernetes API 发现同 namespace 下的 agent Pod。
- 每 10 秒后台刷新一次 agent 数据。
- 优先通过 agent Pod IP 拉取 `http://<podIP>:80/api/node-info`。
- `GET /` 展示汇总 Web UI。
- `GET /api/agents` 返回缓存 JSON。
- `POST /api/refresh` 触发即时刷新。

## 构建前检查

```bash
./scripts/preflight.sh
```

## 构建镜像

默认构建 `linux/amd64` 镜像：

```bash
./scripts/build-image.sh
```

指定 kubectl 版本：

```bash
KUBECTL_VERSION=v1.30.0 ./scripts/build-image.sh
```

构建并推送：

```bash
docker login harbor.rancherlsp.com
PUSH=true ./scripts/build-image.sh
```

多架构构建并推送：

```bash
docker login harbor.rancherlsp.com
PLATFORM=linux/amd64,linux/arm64 PUSH=true ./scripts/build-image.sh
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

查看 agent：

```bash
kubectl get pods -l app.kubernetes.io/name=k8s-tool,app.kubernetes.io/component=agent -o wide
```

查看 server：

```bash
kubectl get pods -l app.kubernetes.io/name=k8s-tool,app.kubernetes.io/component=server -o wide
kubectl get svc k8s-tool-server -o wide
```

访问 server Web UI：

```text
http://<任意节点IP>:<k8s-tool-server NodePort>
```

## 使用其他 Namespace

当前清单默认部署到 `default` namespace。由于 `ClusterRoleBinding` 需要写明 ServiceAccount 所在 namespace，如果要部署到其他 namespace，需要同步修改 `k8s/k8s-tool.yaml` 中：

```yaml
subjects:
  - kind: ServiceAccount
    name: k8s-tool
    namespace: default
```

## 环境变量

- `MODE=agent|server`
- `REFRESH_INTERVAL_SECONDS=10`
- `AGENT_SELECTOR=app.kubernetes.io/name=k8s-tool,app.kubernetes.io/component=agent`
- `AGENT_PORT=80`

## 安全说明

agent 以特权模式运行，适合受控排障场景。server 使用 ServiceAccount 只读权限发现 agent Pod，不需要挂载 admin kubeconfig。
