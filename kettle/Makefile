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
TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# These are the usual GKE variables.
PROJECT       ?= k8s-gubernator
ZONE          ?= us-west1-b
CLUSTER       ?= g8r

get-cluster-credentials:
	kubectl config use-context gke_k8s-gubernator_us-west1-b_g8r || gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

build:
	docker build -t $(IMG):$(TAG) .
	docker tag $(IMG):$(TAG) $(IMG):latest
	echo Built $(IMG):$(TAG) and tagged with $(IMG):latest ;

push: build
	docker push $(IMG):$(TAG)
	docker push $(IMG):latest
	echo Pushed $(IMG):latest and $(IMG):$(TAG)

deploy: get-cluster-credentials
	sed "s/:latest/:$(TAG)/g" deployment.yaml | kubectl apply -f - --record

update: get-cluster-credentials
	kubectl set image deployment/kettle kettle=$(IMG):$(TAG) --record

rollback: get-cluster-credentials
	kubectl rollout undo deployments/kettle


.PHONY: all build push

all: build
