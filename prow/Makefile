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

# YYYYmmdd-commitish
TAG = $(shell date -u +v%Y%m%d)-$(shell git describe --tags --always --dirty)
# HOOK_VERSION is the version of the hook image
HOOK_VERSION             ?= $(TAG)
# SINKER_VERSION is the version of the sinker image
SINKER_VERSION           ?= $(TAG)
# DECK_VERSION is the version of the deck image
DECK_VERSION             ?= $(TAG)
# SPLICE_VERSION is the version of the splice image
SPLICE_VERSION           ?= $(TAG)
# TOT_VERSION is the version of the tot image
TOT_VERSION              ?= $(TAG)
# HOROLOGIUM_VERSION is the version of the horologium image
HOROLOGIUM_VERSION       ?= $(TAG)
# PLANK_VERSION is the version of the plank image
PLANK_VERSION            ?= $(TAG)
# JENKINS-OPERATOR_VERSION is the version of the jenkins-operator image
JENKINS-OPERATOR_VERSION ?= $(TAG)
# TIDE_VERSION is the version of the tide image
TIDE_VERSION             ?= $(TAG)
# CLONEREFS_VERSION is the version of the clonerefs image
CLONEREFS_VERSION        ?= $(TAG)
# INITUPLOAD_VERSION is the version of the initupload image
INITUPLOAD_VERSION       ?= $(TAG)
# GCSUPLOAD_VERSION is the version of the gcsupload image
GCSUPLOAD_VERSION        ?= $(TAG)
# ENTRYPOINT_VERSION is the version of the entrypoint image
ENTRYPOINT_VERSION       ?= $(TAG)
# SIDECAR_VERSION is the version of the sidecar image
SIDECAR_VERSION          ?= $(TAG)

# These are the usual GKE variables.
PROJECT       ?= k8s-prow
BUILD_PROJECT ?= k8s-prow-builds
ZONE          ?= us-central1-f
CLUSTER       ?= prow

# Build and push specific variables.
REGISTRY ?= gcr.io
PUSH     ?= docker push

DOCKER_LABELS=--label io.k8s.prow.git-describe="$(shell git describe --tags --always --dirty)"

update-config: get-cluster-credentials
	kubectl create configmap config --from-file=config=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

update-cat-api-key: get-cluster-credentials
	kubectl create configmap cat-api-key --from-file=api-key=plugins/cat/api-key --dry-run -o yaml | kubectl replace configmap cat-api-key -f -

.PHONY: update-config update-plugins update-cat-api-key

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

get-build-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(BUILD_PROJECT)" --zone="$(ZONE)"

build:
	go install ./cmd/...

test:
	go test -race -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: build test get-cluster-credentials

alpine-image:
	docker build -t "$(REGISTRY)/$(PROJECT)/alpine:$(ALPINE_VERSION)" $(DOCKER_LABELS) cmd/images/alpine
	$(PUSH) "$(REGISTRY)/$(PROJECT)/alpine:$(ALPINE_VERSION)"

git-image: alpine-image
	docker build -t "$(REGISTRY)/$(PROJECT)/git:$(GIT_VERSION)" $(DOCKER_LABELS) cmd/images/git
	$(PUSH) "$(REGISTRY)/$(PROJECT)/git:$(GIT_VERSION)"

.PHONY: alpine-image git-image

branchprotector-image:
	bazel run //prow/cmd/branchprotector:push

branchprotector-cronjob: get-cluster-credentials
	@echo Consider bazel run //prow/cluster:branchprotector_cronjob.apply instead
	kubectl apply -f cluster/branchprotector_cronjob.yaml

.PHONY: branchprotector-image branchprotector-cronjob

hook-image: git-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/hook/hook k8s.io/test-infra/prow/cmd/hook
	docker build -t "$(REGISTRY)/$(PROJECT)/hook:$(HOOK_VERSION)" $(DOCKER_LABELS) cmd/hook
	$(PUSH) "$(REGISTRY)/$(PROJECT)/hook:$(HOOK_VERSION)"

hook-deployment: get-cluster-credentials
	kubectl apply -f cluster/hook_deployment.yaml

hook-service: get-cluster-credentials
	kubectl apply -f cluster/hook_service.yaml

.PHONY: hook-image hook-deployment hook-service

sinker-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/sinker/sinker k8s.io/test-infra/prow/cmd/sinker
	docker build -t "$(REGISTRY)/$(PROJECT)/sinker:$(SINKER_VERSION)" $(DOCKER_LABELS) cmd/sinker
	$(PUSH) "$(REGISTRY)/$(PROJECT)/sinker:$(SINKER_VERSION)"

sinker-deployment: get-cluster-credentials
	kubectl apply -f cluster/sinker_deployment.yaml

.PHONY: sinker-image sinker-deployment

deck-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/deck/deck k8s.io/test-infra/prow/cmd/deck
	docker build -t "$(REGISTRY)/$(PROJECT)/deck:$(DECK_VERSION)" $(DOCKER_LABELS) cmd/deck
	$(PUSH) "$(REGISTRY)/$(PROJECT)/deck:$(DECK_VERSION)"

deck-deployment: get-cluster-credentials
	kubectl apply -f cluster/deck_deployment.yaml

deck-service: get-cluster-credentials
	kubectl apply -f cluster/deck_service.yaml

.PHONY: deck-image deck-deployment deck-service

splice-image: git-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/splice/splice k8s.io/test-infra/prow/cmd/splice
	docker build -t "$(REGISTRY)/$(PROJECT)/splice:$(SPLICE_VERSION)" $(DOCKER_LABELS) cmd/splice
	$(PUSH) "$(REGISTRY)/$(PROJECT)/splice:$(SPLICE_VERSION)"

splice-deployment: get-cluster-credentials
	kubectl apply -f cluster/splice_deployment.yaml

.PHONY: splice-image splice-deployment

tot-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/tot/tot k8s.io/test-infra/prow/cmd/tot
	docker build -t "$(REGISTRY)/$(PROJECT)/tot:$(TOT_VERSION)" $(DOCKER_LABELS) cmd/tot
	$(PUSH) "$(REGISTRY)/$(PROJECT)/tot:$(TOT_VERSION)"

tot-deployment: get-cluster-credentials
	kubectl apply -f cluster/tot_deployment.yaml

tot-service: get-cluster-credentials
	kubectl apply -f cluster/tot_service.yaml

.PHONY: tot-image tot-deployment

horologium-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/horologium/horologium k8s.io/test-infra/prow/cmd/horologium
	docker build -t "$(REGISTRY)/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)" $(DOCKER_LABELS) cmd/horologium
	$(PUSH) "$(REGISTRY)/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)"

horologium-deployment: get-cluster-credentials
	kubectl apply -f cluster/horologium_deployment.yaml

.PHONY: horologium-image horologium-deployment

plank-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/plank/plank k8s.io/test-infra/prow/cmd/plank
	docker build -t "$(REGISTRY)/$(PROJECT)/plank:$(PLANK_VERSION)" $(DOCKER_LABELS) cmd/plank
	$(PUSH) "$(REGISTRY)/$(PROJECT)/plank:$(PLANK_VERSION)"

plank-deployment: get-cluster-credentials
	kubectl apply -f cluster/plank_deployment.yaml

.PHONY: plank-image plank-deployment

jenkins-operator-image: alpine-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/jenkins-operator/jenkins-operator k8s.io/test-infra/prow/cmd/jenkins-operator
	docker build -t "$(REGISTRY)/$(PROJECT)/jenkins-operator:$(JENKINS-OPERATOR_VERSION)" $(DOCKER_LABELS) cmd/jenkins-operator
	$(PUSH) "$(REGISTRY)/$(PROJECT)/jenkins-operator:$(JENKINS-OPERATOR_VERSION)"

jenkins-operator-deployment: get-cluster-credentials
	kubectl apply -f cluster/jenkins_deployment.yaml

pushgateway-deploy: get-cluster-credentials
	kubectl apply -f cluster/pushgateway_deployment.yaml

.PHONY: jenkins-operator-image jenkins-operator-deployment pushgateway-deploy

tide-image: git-image
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cmd/tide/tide k8s.io/test-infra/prow/cmd/tide
	docker build -t "$(REGISTRY)/$(PROJECT)/tide:$(TIDE_VERSION)" $(DOCKER_LABELS) cmd/tide
	$(PUSH) "$(REGISTRY)/$(PROJECT)/tide:$(TIDE_VERSION)"

tide-deployment: get-cluster-credentials
	kubectl apply -f cluster/tide_deployment.yaml

mem-range-deployment: get-build-cluster-credentials
	kubectl apply -f cluster/mem_limit_range.yaml

.PHONY: tide-image tide-deployment mem-range-deployment

clonerefs-image: git-image
	CGO_ENABLED=0 go build -o cmd/clonerefs/clonerefs k8s.io/test-infra/prow/cmd/clonerefs
	docker build -t "$(REGISTRY)/$(PROJECT)/clonerefs:$(CLONEREFS_VERSION)" $(DOCKER_LABELS) cmd/clonerefs
	$(PUSH) "$(REGISTRY)/$(PROJECT)/clonerefs:$(CLONEREFS_VERSION)"

initupload-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/initupload/initupload k8s.io/test-infra/prow/cmd/initupload
	docker build -t "$(REGISTRY)/$(PROJECT)/initupload:$(INITUPLOAD_VERSION)" $(DOCKER_LABELS) cmd/initupload
	$(PUSH) "$(REGISTRY)/$(PROJECT)/initupload:$(INITUPLOAD_VERSION)"

gcsupload-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/gcsupload/gcsupload k8s.io/test-infra/prow/cmd/gcsupload
	docker build -t "$(REGISTRY)/$(PROJECT)/gcsupload:$(GCSUPLOAD_VERSION)" $(DOCKER_LABELS) cmd/gcsupload
	$(PUSH) "$(REGISTRY)/$(PROJECT)/gcsupload:$(GCSUPLOAD_VERSION)"

entrypoint-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/entrypoint/entrypoint k8s.io/test-infra/prow/cmd/entrypoint
	docker build -t "$(REGISTRY)/$(PROJECT)/entrypoint:$(ENTRYPOINT_VERSION)" $(DOCKER_LABELS) cmd/entrypoint
	$(PUSH) "$(REGISTRY)/$(PROJECT)/entrypoint:$(ENTRYPOINT_VERSION)"

sidecar-image: alpine-image
	CGO_ENABLED=0 go build -o cmd/sidecar/sidecar k8s.io/test-infra/prow/cmd/sidecar
	docker build -t "$(REGISTRY)/$(PROJECT)/sidecar:$(SIDECAR_VERSION)" $(DOCKER_LABELS) cmd/sidecar
	$(PUSH) "$(REGISTRY)/$(PROJECT)/sidecar:$(SIDECAR_VERSION)"

.PHONY: clonerefs-image initupload-image gcsupload-image entrypoint-image sidecar-image
