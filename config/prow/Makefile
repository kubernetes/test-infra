# Copyright 2021 The Kubernetes Authors.
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

include ../../hack/Makefile.gcloud.mk

# These are the usual GKE variables.
PROJECT       ?= k8s-prow
CLUSTER       ?= prow
ZONE          ?= us-central1-f

PROJECT_BUILD ?= k8s-prow-builds
CLUSTER_BUILD ?= prow
ZONE_BUILD    ?= us-central1-f

JOB_NAMESPACE ?= test-pods

update-config: get-cluster-credentials
	kubectl create configmap config --from-file=config.yaml=config.yaml --dry-run=client -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins.yaml=plugins.yaml --dry-run=client -o yaml | kubectl replace configmap plugins -f -

deploy-prow: get-cluster-credentials
	kubectl apply --server-side=true -f ./cluster/prowjob-crd/prowjob_customresourcedefinition.yaml
	kubectl apply -f ./cluster/

	# Temporary solution for working around the issue of KES being flaky, see
	# https://github.com/kubernetes/test-infra/issues/24869#issuecomment-1147530320.
	# TODO(chaodaiG): remove this once the above issue is fixed.
	kubectl -n default rollout restart deployment kubernetes-external-secrets

deploy-build: get-build-cluster-credentials
	kubectl apply -f ./cluster/build/

update-boskos-resources: get-build-cluster-credentials
	kubectl create configmap resources --from-file=config=./cluster/build/boskos-resources/boskos-resources.yaml --dry-run=client -o yaml | kubectl --namespace=test-pods replace configmap resources -f -

deploy-monitoring: get-cluster-credentials
	$(MAKE) -C ./cluster/monitoring apply

deploy-all: deploy-prow deploy-build deploy-monitoring

.PHONY: update-config update-plugins deploy-prow deploy-build deploy-monitoring deploy-all

# The targets below are for breaking glass usages, should not run on a regular
# basis

# Run fix-gencred-refresher will generate kubeconfig for "trusted" build cluster
# and store it in GSM, you will need "container.admin" and "secretManager.admin"
# permission on k8s-prow GCP project to perform this action.
# This target is for solving the bootstrapping problem of gencred refresher,
# where it runs in "trusted" build cluster, as a prow job to rotate the
# kubeconfig for "trusted" build cluster along with other build clusters. So
# when the refresher job failed for more than 2 days, prow will stop working
# with "trusted" build cluster and thus not able to schedule gencred refrsher
# prow jobs any more. And this is when the admin of this repo need to run this
# target manually.
fix-gencred-refresher:
	../../hack/make-rules/go-run/arbitrary.sh run ./gencred --config=./config/prow/gencred-config/gencred-config.yaml --gke-filter=projects/k8s-prow/locations/us-central1-f/clusters/prow

.PHONY: fix-gencred-refresher
