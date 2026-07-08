# StableAgent Kubernetes 部署

四个应用服务的 Deployment/Service 清单,对应设计方案 §11 的部署单元:

| 清单 | 说明 |
| --- | --- |
| `namespace.yaml` | `stableagent` 命名空间 |
| `configmap.yaml` | 非敏感配置(服务发现、OTel、对象存储参数) |
| `secret.example.yaml` | 敏感配置示例;生产用 Vault / 云 KMS / ExternalSecrets 替换 |
| `api-service.yaml` / `runtime-worker.yaml` / `tool-gateway.yaml` / `llm-gateway.yaml` | 应用 Deployment + Service,带 `/readyz`、`/healthz` 探针与 Prometheus 抓取注解 |
| `hpa.yaml` | runtime-worker 横向扩缩容 |

## 部署顺序

```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.example.yaml   # 生产替换为真实 Secret 管理
kubectl apply -f api-service.yaml -f runtime-worker.yaml -f tool-gateway.yaml -f llm-gateway.yaml
kubectl apply -f hpa.yaml
```

## 依赖的基础设施

PostgreSQL、Redis、MinIO/S3、OTel Collector、Prometheus、Grafana、Tempo/Jaeger 按环境选择云服务或自建 StatefulSet(本地开发用 `deploy/docker-compose.yml`)。清单中的地址通过 `configmap.yaml` 与 Secret 注入,替换为实际 endpoint 即可。

## 镜像构建

统一 Dockerfile 多目标构建:

```bash
docker build --build-arg SERVICE=api-service --target api-runtime -t registry.example.com/stableagent/api-service:TAG .
docker build --build-arg SERVICE=runtime-worker --target service-runtime -t registry.example.com/stableagent/runtime-worker:TAG .
```
