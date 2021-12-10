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
	kubectl create configmap config --from-file=config.yaml=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins.yaml=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

deploy-prow: get-cluster-credentials
	kubectl apply -f ./cluster/prowjob-crd/legacy/prowjob-schemaless_customresourcedefinition.yaml
	kubectl apply -f ./cluster/

deploy-build: get-build-cluster-credentials
	kubectl apply -f ./cluster/build/

deploy-monitoring: get-cluster-credentials
	$(MAKE) -C ./cluster/monitoring apply

deploy-all: deploy-prow deploy-build deploy-monitoring

.PHONY: update-config update-plugins deploy-prow deploy-build deploy-monitoring deploy-all
