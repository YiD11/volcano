#!/bin/bash
# Copyright 2019 The Volcano Authors.
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

# 在当前进程sleep xxx s，并显示实时进度条
sleep_with_progress() {
    duration=$1
    for ((i = 0; i <= duration; i++)); do
        percent=$((i * 100 / duration))
        bar=$(printf '%0.s=' $(seq 1 $((percent / 2))))
        echo -ne "Progress: [${bar}] ${percent}%\r"
        sleep 1
    done
    echo -ne "\n"
}

# 检查镜像是否存在，不存在则拉取，然后加载到kind集群
load_image_to_kind() {
  local image_name="$1"   # 镜像名，包括repo和tag
  local kind_cluster="$2" # kind集群名称
  
  # 检测 kind 节点的架构
  local node_arch=$(docker exec "${kind_cluster}-control-plane" uname -m 2>/dev/null)
  local platform="linux/amd64"
  if [[ "$node_arch" == "aarch64" ]]; then
    platform="linux/arm64"
  fi

  # 使用docker检查镜像是否存在（检查特定平台的镜像）
  if ! docker images "$image_name" --format "{{.Repository}}:{{.Tag}}" | grep -q "$image_name"; then
    echo "Pulling image: $image_name for platform $platform"
    docker pull --platform "$platform" "$image_name"
  else
    echo "Image $image_name already exists locally."
  fi

  # 规范化镜像名称，添加 docker.io/ 前缀（如果没有 registry 前缀）
  local full_image_name="$image_name"
  if [[ ! "$image_name" =~ ^[^/]+\.[^/]+/ ]]; then
    # 如果镜像名不包含 registry 域名（如 docker.io, quay.io），添加 docker.io/ 前缀
    if [[ "$image_name" =~ / ]]; then
      full_image_name="docker.io/$image_name"
    fi
  fi

  # 将镜像load到指定的kind集群（使用更兼容的方式）
  echo "Loading image $image_name into kind cluster $kind_cluster"
  docker save "$image_name" | docker exec -i "${kind_cluster}-control-plane" ctr --namespace=k8s.io images import --base-name "$full_image_name" -
}