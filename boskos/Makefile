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

PROJECT ?= k8s-prow-builds
ZONE ?= us-central1-f
CLUSTER ?= prow
NAMESPACE ?= test-pods
HUB ?= gcr.io/k8s-testimages

TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

janitor-aws:
	$(MAKE) -C ../maintenance/aws-janitor/cmd/aws-janitor-boskos

images:
	bazel run //boskos:push

janitor-aws-image:
	bazel run //images/builder -- --project=k8s-testimages --scratch-bucket=gs://k8s-testimages-scratch --variant aws-janitor boskos

server-deployment: get-cluster-credentials
	kubectl apply -f deployment.yaml

reaper-deployment: get-cluster-credentials
	kubectl apply -f reaper/deployment.yaml

janitor-deployment: get-cluster-credentials
	kubectl apply -f janitor/deployment.yaml

metrics-deployment: get-cluster-credentials
	kubectl apply -f metrics/deployment.yaml

service: get-cluster-credentials
	kubectl apply -f service.yaml

update-config: get-cluster-credentials
	#TODO: create the resources.yaml and use it instead
	kubectl create configmap resources --from-file=config=resources.yaml --dry-run -o yaml | kubectl --namespace="$(NAMESPACE)" replace configmap resources -f -

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

.PHONY: boskos client reaper janitor janitor-aws metrics images server-image reaper-image janitor-image janitor-aws-image metrics-image server-deployment reaper-deployment janitor-deployment metrics-deployment service update-config get-cluster-credentials
