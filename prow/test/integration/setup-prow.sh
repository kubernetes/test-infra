#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

set -o errexit

CURRENT_REPO="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

CONTEXT="$(kubectl config current-context)"
if [[ -z "${CONTEXT}" ]]; then
  echo "Current kube context cannot be empty"
  exit 1
fi
if [[ "${CONTEXT}" != "kind-kind-prow-integration" ]]; then
  echo "Current kube context is '${CONTEXT}', has to be kind-kind-prow-integration"
  exit 1
fi

echo "Pushing prow images"
bazel run //prow:testimage-push "$@"

echo "Wait until nginx is ready"
kubectl --context=${CONTEXT} wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

echo "Deploy prow components"
kubectl --context=${CONTEXT} create configmap config --from-file=config.yaml=${CURRENT_REPO}/prow/config.yaml --dry-run -oyaml | kubectl apply -f -
kubectl --context=${CONTEXT} apply -f ${CURRENT_REPO}/prow/cluster

echo "Waiting for prow components"
for pod in sinker; do
  kubectl --context=${CONTEXT} wait pod \
    --for=condition=ready \
    --selector=app=${pod} \
    --timeout=180s
done

echo "Push test image to registry"
docker pull busybox
docker tag busybox:latest localhost:5000/busybox:latest
docker push localhost:5000/busybox:latest
