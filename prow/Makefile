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

all: build test


# ALPINE_VERSION is the version of the alpine image
ALPINE_VERSION           ?= 0.1
# GIT_VERSION is the version of the alpine+git image
GIT_VERSION              ?= 0.1
# HOOK_VERSION is the version of the hook image
HOOK_VERSION             ?= 0.181
# SINKER_VERSION is the version of the sinker image
SINKER_VERSION           ?= 0.23
# DECK_VERSION is the version of the deck image
DECK_VERSION             ?= 0.62
# SPLICE_VERSION is the version of the splice image
SPLICE_VERSION           ?= 0.32
# TOT_VERSION is the version of the tot image
TOT_VERSION              ?= 0.5
# HOROLOGIUM_VERSION is the version of the horologium image
HOROLOGIUM_VERSION       ?= 0.17
# PLANK_VERSION is the version of the plank image
PLANK_VERSION            ?= 0.60
# JENKINS-OPERATOR_VERSION is the version of the jenkins-operator image
JENKINS-OPERATOR_VERSION ?= 0.58
# TIDE_VERSION is the version of the tide image
TIDE_VERSION             ?= 0.12

# These are the usual GKE variables.
PROJECT       ?= k8s-prow
BUILD_PROJECT ?= k8s-prow-builds
ZONE          ?= us-central1-f
CLUSTER       ?= prow

# Build and push specific variables.
REGISTRY ?= gcr.io
PUSH     ?= gcloud docker -- push

DOCKER_LABELS=--label io.k8s.prow.git-describe="$(shell git describe --tags --always --dirty)"

update-config: get-cluster-credentials
	kubectl create configmap config --from-file=config=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

get-build-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(BUILD_PROJECT)" --zone="$(ZONE)"

build:
	go install ./cmd/...

test:
	go test -race -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: update-config update-plugins build test get-cluster-credentials

alpine-image:
	docker build -t "$(REGISTRY)/$(PROJECT)/alpine:$(ALPINE_VERSION)" $(DOCKER_LABELS) cmd/images/alpine
	$(PUSH) "$(REGISTRY)/$(PROJECT)/alpine:$(ALPINE_VERSION)"

git-image: alpine-image
	docker build -t "$(REGISTRY)/$(PROJECT)/git:$(GIT_VERSION)" $(DOCKER_LABELS) cmd/images/git
	$(PUSH) "$(REGISTRY)/$(PROJECT)/git:$(GIT_VERSION)"

hook-image: git-image
	CGO_ENABLED=0 go build -o cmd/hook/hook k8s.io/test-infra/prow/cmd/hook
	docker build -t "$(REGISTRY)/$(PROJECT)/hook:$(HOOK_VERSION)" $(DOCKER_LABELS) cmd/hook
	$(PUSH) "$(REGISTRY)/$(PROJECT)/hook:$(HOOK_VERSION)"

hook-deployment: get-cluster-credentials
	kubectl apply -f cluster/hook_deployment.yaml

hook-service: get-cluster-credentials
	kubectl apply -f cluster/hook_service.yaml

sinker-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/sinker/sinker k8s.io/test-infra/prow/cmd/sinker
	docker build -t "$(REGISTRY)/$(PROJECT)/sinker:$(SINKER_VERSION)" $(DOCKER_LABELS) cmd/sinker
	$(PUSH) "$(REGISTRY)/$(PROJECT)/sinker:$(SINKER_VERSION)"

sinker-deployment: get-cluster-credentials
	kubectl apply -f cluster/sinker_deployment.yaml

deck-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/deck/deck k8s.io/test-infra/prow/cmd/deck
	docker build -t "$(REGISTRY)/$(PROJECT)/deck:$(DECK_VERSION)" $(DOCKER_LABELS) cmd/deck
	$(PUSH) "$(REGISTRY)/$(PROJECT)/deck:$(DECK_VERSION)"

deck-deployment: get-cluster-credentials
	kubectl apply -f cluster/deck_deployment.yaml

deck-service: get-cluster-credentials
	kubectl apply -f cluster/deck_service.yaml

splice-image: git-image
	CGO_ENABLED=0 go build -o cmd/splice/splice k8s.io/test-infra/prow/cmd/splice
	docker build -t "$(REGISTRY)/$(PROJECT)/splice:$(SPLICE_VERSION)" $(DOCKER_LABELS) cmd/splice
	$(PUSH) "$(REGISTRY)/$(PROJECT)/splice:$(SPLICE_VERSION)"

splice-deployment: get-cluster-credentials
	kubectl apply -f cluster/splice_deployment.yaml

tot-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/tot/tot k8s.io/test-infra/prow/cmd/tot
	docker build -t "$(REGISTRY)/$(PROJECT)/tot:$(TOT_VERSION)" $(DOCKER_LABELS) cmd/tot
	$(PUSH) "$(REGISTRY)/$(PROJECT)/tot:$(TOT_VERSION)"

tot-deployment: get-cluster-credentials
	kubectl apply -f cluster/tot_deployment.yaml

tot-service: get-cluster-credentials
	kubectl apply -f cluster/tot_service.yaml

horologium-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/horologium/horologium k8s.io/test-infra/prow/cmd/horologium
	docker build -t "$(REGISTRY)/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)" $(DOCKER_LABELS) cmd/horologium
	$(PUSH) "$(REGISTRY)/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)"

horologium-deployment: get-cluster-credentials
	kubectl apply -f cluster/horologium_deployment.yaml

plank-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/plank/plank k8s.io/test-infra/prow/cmd/plank
	docker build -t "$(REGISTRY)/$(PROJECT)/plank:$(PLANK_VERSION)" $(DOCKER_LABELS) cmd/plank
	$(PUSH) "$(REGISTRY)/$(PROJECT)/plank:$(PLANK_VERSION)"

plank-deployment: get-cluster-credentials
	kubectl apply -f cluster/plank_deployment.yaml

jenkins-operator-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/jenkins-operator/jenkins-operator k8s.io/test-infra/prow/cmd/jenkins-operator
	docker build -t "$(REGISTRY)/$(PROJECT)/jenkins-operator:$(JENKINS-OPERATOR_VERSION)" $(DOCKER_LABELS) cmd/jenkins-operator
	$(PUSH) "$(REGISTRY)/$(PROJECT)/jenkins-operator:$(JENKINS-OPERATOR_VERSION)"

jenkins-operator-deployment: get-cluster-credentials
	kubectl apply -f cluster/jenkins_deployment.yaml

push-gateway-deploy: get-cluster-credentials
	kubectl apply -f cluster/push_gateway.yaml

tide-image: git-image
	CGO_ENABLED=0 go build -o cmd/tide/tide k8s.io/test-infra/prow/cmd/tide
	docker build -t "$(REGISTRY)/$(PROJECT)/tide:$(TIDE_VERSION)" $(DOCKER_LABELS) cmd/tide
	$(PUSH) "$(REGISTRY)/$(PROJECT)/tide:$(TIDE_VERSION)"

tide-deployment: get-cluster-credentials
	kubectl apply -f cluster/tide_deployment.yaml

mem-range-deployment: get-build-cluster-credentials
	kubectl apply -f cluster/mem_limit_range.yaml

.PHONY: hook-image hook-deployment hook-service sinker-image sinker-deployment deck-image deck-deployment deck-service splice-image splice-deployment tot-image tot-service tot-deployment horologium-image horologium-deployment plank-image plank-deployment jenkins-operator-image jenkins-operator-deployment tide-image tide-deployment mem-range-deployment
