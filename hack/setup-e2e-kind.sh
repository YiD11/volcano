#!/bin/bash

#
# Copyright 2021 The Volcano Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# This script sets up an e2e testing environment without running tests
# You can manually run tests after the cluster is ready

export VK_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/..
export VC_BIN=${VK_ROOT}/${BIN_DIR}/${BIN_OSARCH}
export TAG=${TAG:-$(git -C "${VK_ROOT}" rev-parse --verify HEAD 2>/dev/null || echo "latest")}
export IMAGE_PREFIX=${IMAGE_PREFIX:-volcanosh}
export LOG_LEVEL=3
export E2E_TYPE=${E2E_TYPE:-"ALL"}
export ARTIFACTS_PATH=${ARTIFACTS_PATH:-"${VK_ROOT}/volcano-e2e-logs"}
mkdir -p "$ARTIFACTS_PATH"

NAMESPACE=${NAMESPACE:-volcano-system}
CLUSTER_NAME=${CLUSTER_NAME:-integration}

export CLUSTER_CONTEXT=("--name" "${CLUSTER_NAME}")

export KIND_OPT=${KIND_OPT:="--config ${VK_ROOT}/hack/e2e-kind-config.yaml"}
export SCHEDULINGPLUGIN_CONFIG=${SCHEDULINGPLUGIN_CONFIG:-"${VK_ROOT}/test/e2e/schedulingplugin/volcano-scheduler-ci.conf"}

# Parse verbose flag
VERBOSE=0
for arg in "$@"; do
  case $arg in
    -v|--verbose)
      VERBOSE=1
      shift
      ;;
  esac
done

# kwok node config
export KWOK_NODE_CPU=${KWOK_NODE_CPU:-2}      # 8 cores
export KWOK_NODE_MEMORY=${KWOK_NODE_MEMORY:-2Gi}  # 8GB

# create kwok node
function create-kwok-node() {
  local node_index=$1
  
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    kwok.x-k8s.io/node: fake
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: kwok-node-${node_index}
    kubernetes.io/os: linux
    kubernetes.io/role: agent
    node-role.kubernetes.io/agent: ""
    type: kwok
  name: kwok-node-${node_index}
spec:
  taints:
  - effect: NoSchedule
    key: kwok.x-k8s.io/node
    value: fake
status:
  capacity:
    cpu: "${KWOK_NODE_CPU}"
    memory: "${KWOK_NODE_MEMORY}"
    pods: "110"
  allocatable:
    cpu: "${KWOK_NODE_CPU}"
    memory: "${KWOK_NODE_MEMORY}"
    pods: "110"
EOF
}

# install kwok nodes
function install-kwok-nodes() {
  local node_count=$1
  for i in $(seq 0 $((node_count-1))); do
    create-kwok-node $i
  done
}

function install-volcano {
  install-helm

  # judge crd version
  major=$(kubectl version --output yaml | awk '/serverVersion/,0' |grep -E 'major:' | awk '{print $2}' | tr "\"" " ")
  minor=$(kubectl version --output yaml | awk '/serverVersion/,0' |grep -E 'minor:' | awk '{print $2}' | tr "\"" " ")
  crd_version="v1"
  # if k8s version less than v1.18, crd version use v1beta
  if [ "$major" -le "1" ]; then
    if [ "$minor" -lt "18" ]; then
      crd_version="v1beta1"
    fi
  fi

  if [[ ! -f "${SCHEDULINGPLUGIN_CONFIG}" ]]; then
    echo "ERROR: scheduler config not found: ${SCHEDULINGPLUGIN_CONFIG}"
    exit 1
  fi
  SCHEDULER_CONFIG_OVERRIDE=$(sed 's/^/    /' "${SCHEDULINGPLUGIN_CONFIG}")

  echo "Ensure create namespace"
  kubectl apply -f installer/namespace.yaml

case ${E2E_TYPE} in
"ADMISSION_POLICY")
  echo "Install volcano chart with crd version $crd_version and VAP/MAP enabled"
  cat <<EOF | helm install ${CLUSTER_NAME} installer/helm/chart/volcano \
  --namespace ${NAMESPACE} \
  --kubeconfig ${KUBECONFIG} \
  --values - \
  --wait
basic:
  image_pull_policy: IfNotPresent
  image_tag_version: ${TAG}
  scheduler_config_file: config/volcano-scheduler-ci.conf
  crd_version: ${crd_version}

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
  enabled_admissions: ""
  vap_enable: true
  map_enable: true
  ignored_provisioners: ${IGNORED_PROVISIONERS:-""}
EOF
  ;;
"ADMISSION_WEBHOOK")
  echo "Install volcano chart with crd version $crd_version and all webhook"
  cat <<EOF | helm install ${CLUSTER_NAME} installer/helm/chart/volcano \
  --namespace ${NAMESPACE} \
  --kubeconfig ${KUBECONFIG} \
  --values - \
  --wait
basic:
  image_pull_policy: IfNotPresent
  image_tag_version: ${TAG}
  scheduler_config_file: config/volcano-scheduler-ci.conf
  crd_version: ${crd_version}

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
  vap_enable: false
  map_enable: false
  ignored_provisioners: ${IGNORED_PROVISIONERS:-""}
EOF
  ;;
*)
  echo "Install volcano chart with crd version $crd_version"
  cat <<EOF | helm install ${CLUSTER_NAME} installer/helm/chart/volcano \
  --namespace ${NAMESPACE} \
  --kubeconfig ${KUBECONFIG} \
  --values - \
  --wait
basic:
  image_pull_policy: IfNotPresent
  image_tag_version: ${TAG}
  scheduler_config_file: config/volcano-scheduler-ci.conf
  crd_version: ${crd_version}

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
  ;;
esac
}

function uninstall-volcano {
  helm uninstall "${CLUSTER_NAME}" -n ${NAMESPACE}
}

function cleanup {
  uninstall-volcano

  echo "Running kind: [kind delete cluster ${CLUSTER_CONTEXT[*]}]"
  kind delete cluster "${CLUSTER_CONTEXT[@]}"
}

echo $* | grep -E -q "\-\-help|\-h"
if [[ $? -eq 0 ]]; then
  echo "Setup E2E Kind Cluster for Manual Testing

Usage: $0 [options]

This script sets up a Kind cluster with Volcano installed, but does not run tests.
You can manually run tests after the cluster is ready.

Options:
  -h, --help            Show this help message
  -v, --verbose         Show detailed test command examples after setup

Environment Variables:
  CLUSTER_NAME          Custom cluster name (default: integration)
  KIND_OPT              Kind options other than --name
  E2E_TYPE              Type of e2e environment setup (default: ALL)
                        Options: ALL, ADMISSION_POLICY, ADMISSION_WEBHOOK
  FEATURE_GATES         Feature gates for scheduler (optional)
  SCHEDULINGPLUGIN_CONFIG  Scheduler config path for schedulingplugin e2e
  KEEP_CLUSTER          Set to 1 to keep cluster running (default: 1)

Examples:
  # Setup default cluster
  ./hack/setup-e2e-kind.sh

  # Setup with custom cluster name
  export CLUSTER_NAME=my-test
  ./hack/setup-e2e-kind.sh

  # Setup with admission policy enabled
  export E2E_TYPE=ADMISSION_POLICY
  ./hack/setup-e2e-kind.sh

After setup, you can run tests manually:
  # Example: Run jobp tests
  KUBECONFIG=\${HOME}/.kube/config ginkgo -r --nodes=4 --compilers=4 --randomize-all --randomize-suites --fail-on-pending --cover --trace --race --slow-spec-threshold='30s' --progress ./test/e2e/jobp/

  # Example: Run scheduling base tests
  KUBECONFIG=\${HOME}/.kube/config ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/schedulingbase/

To cleanup:
  kind delete cluster --name ${CLUSTER_NAME}
  # or run:
  helm uninstall ${CLUSTER_NAME} -n volcano-system
  kind delete cluster --name ${CLUSTER_NAME}
"
  exit 0
fi

source "${VK_ROOT}/hack/lib/install.sh"

# E2E test images used by test/e2e tests
E2E_TEST_IMAGES=(
  "nginx:1.29.3-alpine"
  "busybox:1.37"
)

# Pull and load e2e test images into the kind cluster
function load-e2e-test-images {
  echo
  echo "Loading e2e test images into kind cluster"
  
  # Detect architecture for pulling the correct platform image
  ARCH=$(uname -m)
  case ${ARCH} in
    x86_64) PLATFORM="linux/amd64" ;;
    aarch64|arm64) PLATFORM="linux/arm64" ;;
    *) PLATFORM="linux/amd64" ;;
  esac
  echo "Detected architecture: ${ARCH}, using platform: ${PLATFORM}"

  # Get all kind nodes
  KIND_NODES=$(kind get nodes "${CLUSTER_CONTEXT[@]}" 2>/dev/null)
  if [[ -z "${KIND_NODES}" ]]; then
    echo -e "\033[33mWARNING\033[0m: No kind nodes found, skipping e2e image loading"
    return
  fi

  for image in "${E2E_TEST_IMAGES[@]}"; do
    echo "Pulling image: ${image} (platform: ${PLATFORM})"
    if ! docker image inspect "${image}" > /dev/null 2>&1; then
      docker pull --platform "${PLATFORM}" "${image}" || {
        echo -e "\033[33mWARNING\033[0m: Failed to pull ${image}, tests using this image may fail"
        continue
      }
    fi
    
    echo "Loading image into kind cluster nodes: ${image}"
    for node in ${KIND_NODES}; do
      echo "  -> Loading into node: ${node}"
      docker save "${image}" | docker exec -i "${node}" ctr --namespace=k8s.io images import - > /dev/null 2>&1 || {
        echo -e "\033[33mWARNING\033[0m: Failed to load ${image} into node ${node}"
      }
    done
  done
  
}

echo "========================================="
echo "Setting up E2E Kind Cluster for Volcano"
echo "========================================="

check-prerequisites
kind-up-cluster

# Load e2e test images into the kind cluster
load-e2e-test-images

install-kwok-with-helm

if [[ -z ${KUBECONFIG+x} ]]; then
    export KUBECONFIG="${HOME}/.kube/config"
fi

install-volcano

# Optionally install kwok nodes for hypernode testing
if [[ "${E2E_TYPE}" == "HYPERNODE" ]]; then
    echo "Creating 8 kwok nodes for 3-tier topology"
    install-kwok-nodes 8
fi

echo ""
echo "========================================="
echo "✅ E2E Cluster Setup Complete!"
echo "========================================="
echo ""
echo "Cluster Name: ${CLUSTER_NAME}"
echo "Namespace: ${NAMESPACE}"
echo "KUBECONFIG: ${KUBECONFIG}"
echo "Image Tag: ${TAG}"
echo ""

if [[ $VERBOSE -eq 1 ]]; then
  echo "You can now manually run e2e tests, for example:"
  echo ""
  echo "  # Run all jobp tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --nodes=4 --compilers=4 --randomize-all --randomize-suites --fail-on-pending --cover --trace --race --slow-spec-threshold='30s' --progress ./test/e2e/jobp/"
  echo ""
  echo "  # Run scheduling base tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/schedulingbase/"
  echo ""
  echo "  # Run scheduling action tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/schedulingaction/"
  echo ""
  echo "  # Run jobseq tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/jobseq/"
  echo ""
  echo "  # Run vcctl tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/vcctl/"
  echo ""
  echo "  # Run cronjob tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/cronjob/"
  echo ""
  echo "  # Run admission tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/admission/"
  echo ""
  echo "  # Run hypernode tests"
  echo "  KUBECONFIG=${KUBECONFIG} ginkgo -r --slow-spec-threshold='30s' --progress ./test/e2e/hypernode/"
  echo ""
fi

echo "To cleanup the cluster:"
echo "  kind delete cluster --name ${CLUSTER_NAME}"
echo ""
echo "Use -v or --verbose flag to see detailed test command examples"
echo ""
