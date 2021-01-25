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

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

CONTEXT="$(kubectl config current-context)"
if [[ -z "${CONTEXT}" ]]; then
  echo "Current kube context cannot be empty"
  exit 1
fi
if [[ "${CONTEXT}" != "kind-kind-prow-integration" ]]; then
  echo "Current kube context is '${CONTEXT}', has to be kind-kind-prow-integration"
  exit 1
fi

for app in sinker crier fakeghserver; do
  kubectl delete deployment -l app=${app}
  kubectl delete pods -l app=${app}
done

echo "Pushing prow images"
for retry_count in 1 2 3; do
  if bazel run //prow:testimage-push "$@"; then
    echo "Succeeded pushing test images"
    break
  fi
  echo "*************** Test Image Push Failed, Retrying ${retry_count} ***************"
done

echo "Deploy prow components"
# An unfortunately workaround for https://github.com/kubernetes/ingress-nginx/issues/5968.
kubectl delete -A ValidatingWebhookConfiguration ingress-nginx-admission
kubectl --context=${CONTEXT} create configmap config --from-file=config.yaml=${CURRENT_DIR}/prow/config.yaml --dry-run -oyaml | kubectl apply -f -
kubectl --context=${CONTEXT} apply -f ${CURRENT_DIR}/prow/cluster

echo "Wait until nginx is ready"
kubectl --context=${CONTEXT} wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

echo "Waiting for prow components"
for app in sinker crier fakeghserver; do
  kubectl --context=${CONTEXT} wait pod \
    --for=condition=ready \
    --selector=app=${app} \
    --timeout=180s
done

echo "Push test image to registry"
docker pull gcr.io/k8s-prow/alpine
docker tag gcr.io/k8s-prow/alpine:latest localhost:5000/alpine:latest
docker push localhost:5000/alpine:latest
