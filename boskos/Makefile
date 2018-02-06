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

TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

boskos:
	go build k8s.io/test-infra/boskos/

client:
	go build -o client/client k8s.io/test-infra/boskos/client/

reaper:
	go build -o reaper/reaper k8s.io/test-infra/boskos/reaper/

janitor:
	go build -o janitor/janitor k8s.io/test-infra/boskos/janitor/

metrics:
	go build -o metrics/metrics k8s.io/test-infra/boskos/metrics/

server-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o boskos k8s.io/test-infra/boskos/
	docker build -t "$(HUB)/boskos:$(TAG)" .
	docker push "$(HUB)/boskos:$(TAG)"
	rm boskos

reaper-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reaper/reaper k8s.io/test-infra/boskos/reaper/
	docker build -t "$(HUB)/reaper:$(TAG)" reaper
	docker push "$(HUB)/reaper:$(TAG)"
	rm reaper/reaper

janitor-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o janitor/janitor k8s.io/test-infra/boskos/janitor/
	docker build --no-cache -t "$(HUB)/janitor:$(TAG)" janitor
	docker push "$(HUB)/janitor:$(TAG)"
	rm janitor/janitor

metrics-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o metrics/metrics k8s.io/test-infra/boskos/metrics/
	docker build -t "$(HUB)/metrics:$(TAG)" metrics
	docker push "$(HUB)/metrics:$(TAG)"
	rm metrics/metrics

server-deployment:
	kubectl apply -f deployment.yaml

reaper-deployment:
	kubectl apply -f reaper/deployment.yaml

janitor-deployment:
	kubectl apply -f janitor/deployment.yaml

metrics-deployment:
	kubectl apply -f metrics/deployment.yaml

service:
	kubectl apply -f service.yaml

update-config: get-cluster-credentials
	#TODO: create the resources.yaml and use it instead
	kubectl create configmap resources --from-file=config=resources.json --dry-run -o yaml | kubectl --namespace="$(NAMESPACE)" replace configmap resources -f -

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

.PHONY: boskos client reaper janitor metrics server-image reaper-image janitor-image metrics-image server-deployment reaper-deployment janitor-deployment metrics-deployment service update-config get-cluster-credentials
