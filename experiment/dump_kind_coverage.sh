#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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

# Dump coverage files from a kind node.
# Usage: dump_kind_coverage.sh [cluster] [destination]

# Executes a command inside the node running the kind cluster named ${cluster}.
exec_in_node() {
   docker exec $(docker ps -f name=kind-${cluster}-control-plane --format '{{.ID}}') "$@"
}

# Executes a command in container $1 in the kind cluster named ${cluster}
exec_in_container() {
  local container="$1"
  shift
  exec_in_node bash -c "docker exec \"\$(docker ps -f label=io.kubernetes.container.name=${container} --format '{{.ID}}')\" $*"
}

# Produces the coverage log for service $1 in the kind cluster named ${cluster}
get_container_log() {
  local service="$1"
  exec_in_container "${service}" cat "/tmp/k8s-${service}.cov"
}

# Dumps logs from the cluster named $1 to $2.
dump_logs() {
  local cluster="$1"
  local destination="$2"
  get_container_log "kube-apiserver" > "${destination}/kube-apiserver.cov"
  get_container_log "kube-scheduler" > "${destination}/kube-scheduler.cov"
  get_container_log "kube-controller-manager" > "${destination}/kube-controller-manager.cov"
  get_container_log "kube-proxy" > "${destination}/kube-proxy.cov"
  exec_in_node cat /tmp/k8s-kubelet.cov > "${destination}/kubelet.cov"
}

dump_logs "$@"
