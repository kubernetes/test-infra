#!/usr/bin/env bash
set -o errexit

CURRENT_REPO="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

CONTEXT="$(kubectl config current-context)"
if [[ -z "${CONTEXT}" ]]; then
  echo "Current kube context cannot be empty"
  exit 1
fi
if [[ "${CONTEXT}" != "kind-kind" ]]; then
  echo "Current kube context is '${CONTEXT}', has to be kind-kind"
  exit 1
fi

echo "Pushing prow images"
bazel run //prow:testimage-push

echo "Wait until nginx is ready"
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

echo "Deploy prow components"
kubectl create configmap config --from-file=config.yaml=${CURRENT_REPO}/prow/config.yaml --dry-run -oyaml | kubectl apply -f -
kubectl create configmap job-config --from-file=config.yaml=${CURRENT_REPO}//prow/job-config.yaml --dry-run -oyaml | kubectl apply -f -
kubectl apply -f ${CURRENT_REPO}/prow/cluster
