#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
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

KUBE_ROOT=${KUBE_ROOT:-"../../kubernetes"}
K8S_VERSION=$(cat ${KUBE_ROOT}/bazel-out/stable-status.txt | grep STABLE_DOCKER_TAG | awk '{print $2}')

TMPDIR=$(mktemp -d -p /tmp)
echo "Config dir lives at ${TMPDIR}"
CONTAINER=$(docker run -d --privileged=true --security-opt seccomp:unconfined --cap-add=SYS_ADMIN \
  -v /lib/modules:/lib/modules:ro,rshared -v /sys/fs/cgroup:/sys/fs/cgroup:ro,rshared -v ${TMPDIR}:/var/kubernetes:rw,rshared \
  k8s.gcr.io/dind-cluster-amd64:${K8S_VERSION})
echo "The cluster lives in container ${CONTAINER}"
