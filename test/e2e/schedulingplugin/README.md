# schedulingplugin E2E 说明书

本文档说明如何在本地启动模拟集群与所需环境，并执行 `test/e2e/schedulingplugin` 下的端到端测试。

## 目标与范围

- 使用 kind 作为模拟集群
- 使用 KWOK 提供模拟节点
- 使用 Helm 安装 Volcano，并加载本地构建镜像
- 运行 `schedulingplugin` 相关 E2E 测试

## 前置条件

需要以下工具（脚本会自动安装部分依赖，但仍需具备 Go 与 Docker 环境）：

- Go（用于 `go install` 安装 kind 与 ginkgo）
- Docker（用于构建与加载镜像）
- kubectl
- kind
- helm
- ginkgo

可用以下命令确认版本是否可用：

```bash
go version
docker version
kubectl version --client
kind version
helm version
ginkgo version
```

## 1. 构建 Volcano 镜像

`hack/lib/install.sh` 会在创建集群时加载以下镜像：

- `${IMAGE_PREFIX}/vc-controller-manager:${TAG}`
- `${IMAGE_PREFIX}/vc-scheduler:${TAG}`
- `${IMAGE_PREFIX}/vc-webhook-manager:${TAG}`

请确保镜像已经在本地构建完成，并与后续步骤中的 `IMAGE_PREFIX`/`TAG` 保持一致：

```bash
export IMAGE_PREFIX=volcanosh
export TAG=$(git rev-parse --verify HEAD)

make images
```

## 2. 创建 kind 集群

使用项目内置的 kind 配置启动集群：

```bash
export CLUSTER_NAME=integration
kind create cluster --name "${CLUSTER_NAME}" --config /Users/yid11/project/crater/volcano/hack/e2e-kind-config.yaml
```

加载本地镜像到 kind 控制面节点（与 `hack/lib/install.sh` 行为一致）：

```bash
kind load docker-image "${IMAGE_PREFIX}/vc-controller-manager:${TAG}" --name "${CLUSTER_NAME}" --nodes "${CLUSTER_NAME}-control-plane"
kind load docker-image "${IMAGE_PREFIX}/vc-scheduler:${TAG}" --name "${CLUSTER_NAME}" --nodes "${CLUSTER_NAME}-control-plane"
kind load docker-image "${IMAGE_PREFIX}/vc-webhook-manager:${TAG}" --name "${CLUSTER_NAME}" --nodes "${CLUSTER_NAME}-control-plane"
```

## 3. 安装 KWOK（模拟节点）

与 `hack/lib/install.sh` 中的安装逻辑一致：

```bash
helm repo add kwok https://kwok.sigs.k8s.io/charts/
helm repo update
helm upgrade --namespace kube-system --install kwok kwok/kwok
helm upgrade --install kwok kwok/stage-fast
kubectl delete stage pod-complete
```

如需模拟节点数量，可参考 `hack/run-e2e-kind.sh` 中的 KWOK 逻辑后自行创建节点。

## 4. 安装 Volcano（Helm）

`hack/setup-e2e-kind.sh` 会通过 `custom.scheduler_config_override` 注入 `test/e2e/schedulingplugin/volcano-scheduler-ci.conf`，并生成 `integration-scheduler-configmap`，E2E 用例会动态修改它。
如需切换配置文件，可设置 `SCHEDULINGPLUGIN_CONFIG`。

根据 K8s 版本选择 CRD 版本：若 serverVersion < 1.18 使用 `v1beta1`，否则使用 `v1`。

```bash
export NAMESPACE=volcano-system
export CRD_VERSION=v1

SCHEDULINGPLUGIN_CONFIG=/Users/yid11/project/crater/volcano/test/e2e/schedulingplugin/volcano-scheduler-ci.conf
SCHEDULER_CONFIG_OVERRIDE=$(sed 's/^/    /' "${SCHEDULINGPLUGIN_CONFIG}")

helm install "${CLUSTER_NAME}" /Users/yid11/project/crater/volcano/installer/helm/chart/volcano \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --values - \
  --wait <<EOF
basic:
  image_pull_policy: IfNotPresent
  image_tag_version: ${TAG}
  scheduler_config_file: config/volcano-scheduler-ci.conf
  crd_version: ${CRD_VERSION}

custom:
  scheduler_config_override: |
${SCHEDULER_CONFIG_OVERRIDE}
  scheduler_log_level: 5
  admission_tolerations:
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoSchedule"
    - key: "node-role.kubernetes.io/master"
      operator: "Exists"
      effect: "NoSchedule"
  controller_tolerations:
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoSchedule"
    - key: "node-role.kubernetes.io/master"
      operator: "Exists"
      effect: "NoSchedule"
  scheduler_tolerations:
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoSchedule"
    - key: "node-role.kubernetes.io/master"
      operator: "Exists"
      effect: "NoSchedule"
  default_ns:
    node-role.kubernetes.io/control-plane: ""
  scheduler_feature_gates: ${FEATURE_GATES}
  enabled_admissions: "/pods/mutate,/queues/mutate,/podgroups/mutate,/jobs/mutate,/jobs/validate,/jobflows/validate,/pods/validate,/queues/validate,/podgroups/validate,/hypernodes/validate,/cronjobs/validate"
  ignored_provisioners: ${IGNORED_PROVISIONERS:-""}
EOF
```

确认 Volcano 组件已经 Ready：

```bash
kubectl -n "${NAMESPACE}" get pods
```

## 5. 运行 schedulingplugin E2E

```bash
export KUBECONFIG="${HOME}/.kube/config"
cd /Users/yid11/project/crater/volcano
KUBECONFIG="${KUBECONFIG}" ginkgo -v -r --slow-spec-threshold='30s' --progress ./test/e2e/schedulingplugin/
```

如需聚焦单个用例，可加 `--focus`：

```bash
KUBECONFIG="${KUBECONFIG}" ginkgo -v -r --focus="Ex-Priority" ./test/e2e/schedulingplugin/
```

## 6. 清理环境

```bash
helm uninstall "${CLUSTER_NAME}" -n "${NAMESPACE}"
kind delete cluster --name "${CLUSTER_NAME}"
```
