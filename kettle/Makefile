# Copyright 2016 The Kubernetes Authors.
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

IMG = gcr.io/k8s-testimages/kettle
TAG := $(shell date +v%Y%m%d%S)-$(shell git describe --tags --always --dirty)

# These are the usual GKE variables.
PROJECT       ?= k8s-gubernator
ZONE          ?= us-west1-b
CLUSTER       ?= g8r

get-cluster-credentials:
	kubectl config use-context gke_k8s-gubernator_us-west1-b_g8r || gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

push-prod:
	bazel run //images/builder -- --project=k8s-testimages --scratch-bucket=gs://k8s-testimages-scratch --build-dir=. kettle/

push:
	bazel run //images/builder -- --project=k8s-testimages --allow-dirty --build-dir=. kettle/

deploy: get-cluster-credentials
	sed "s/:latest/:$(TAG)/g" deployment.yaml | kubectl apply -f - --record

update: get-cluster-credentials
	kubectl set image deployment/kettle kettle=$(IMG):$(TAG) --record

deploy-staging: get-cluster-credentials
	sed "s/:latest/:$(TAG)/g" deployment-staging.yaml | kubectl apply -f - --record

update-staging: get-cluster-credentials
	kubectl set image deployment/kettle-staging kettle-staging=$(IMG):$(TAG) --record

rollback: get-cluster-credentials
	kubectl rollout undo deployments/kettle


.PHONY: all push
