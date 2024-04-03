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

include ../Makefile.base.mk
# Makefile.base.mk requires REPO_ROOT to be set
REPO_ROOT:=${CURDIR}/..

IMG = gcr.io/k8s-testimages/kettle
TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# These are the usual GKE variables.
PROJECT       ?= kubernetes-public
ZONE          ?= us-central1
CLUSTER       ?= aaa

get-cluster-credentials:
	kubectl config use-context gke_kubernetes-public_us-central1_aaa || gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

push-prod:
	../hack/make-rules/go-run/arbitrary.sh run ./images/builder --project=k8s-staging-infra-tools --scratch-bucket=gs://k8s-testimages-scratch --build-dir=. kettle/

push:
	../hack/make-rules/go-run/arbitrary.sh run ./images/builder --project=k8s-staging-infra-tools --allow-dirty --build-dir=. kettle/

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

test-make-db: ensure-py-requirements3
	../hack/run-in-python-container.sh ./kettle/make_db_test.py

test-model: ensure-py-requirements3
	../hack/run-in-python-container.sh ./kettle/model_test.py

test-stream: ensure-py-requirements3
	../hack/run-in-python-container.sh ./kettle/stream_test.py

test-make-json: ensure-py-requirements3
	../hack/run-in-python-container.sh ./kettle/make_json_test.py

test: test-make-db test-model test-stream test-make-json

.PHONY: all push test-make-db
