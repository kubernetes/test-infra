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
GIT_VERSION              ?= 0.2

# These are the usual GKE variables.
PROJECT       ?= k8s-prow
BUILD_PROJECT ?= k8s-prow-builds
ZONE          ?= us-central1-f
CLUSTER       ?= prow

# Build and push specific variables.
REGISTRY ?= gcr.io
PUSH     ?= docker push

export PROW_REPO_OVERRIDE ?= $(REGISTRY)/$(PROJECT)

DOCKER_LABELS:=--label io.k8s.prow.git-describe="$(shell git describe --tags --always --dirty)"

update-config: get-cluster-credentials
	kubectl create configmap config --from-file=config.yaml=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins.yaml=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

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

bazel-release-push:
	@echo Please use prow/bump.sh or bazel run //prow:release-push to build images
	@echo See https://bazel.build/ for install options.
	@echo Be sure to setup authentication: https://github.com/bazelbuild/rules_docker#authentication
	@echo Also run gcloud auth application-default login
	bazel run //prow:release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

.PHONY: bazel-release-push

branchprotector-image: bazel-release-push

branchprotector-cronjob: get-cluster-credentials
	@echo Consider bazel run //config/prow/cluster:branchprotector_cronjob.apply instead
	kubectl apply -f cluster/branchprotector_cronjob.yaml

.PHONY: branchprotector-image branchprotector-cronjob

hook-image: bazel-release-push

hook-deployment: get-cluster-credentials
	kubectl apply -f cluster/hook_deployment.yaml

hook-service: get-cluster-credentials
	kubectl apply -f cluster/hook_service.yaml

.PHONY: hook-image hook-deployment hook-service

sinker-image: bazel-release-push

sinker-deployment: get-cluster-credentials
	kubectl apply -f cluster/sinker_deployment.yaml

.PHONY: sinker-image sinker-deployment

deck-image: bazel-release-push

deck-deployment: get-cluster-credentials
	kubectl apply -f cluster/deck_deployment.yaml

deck-service: get-cluster-credentials
	kubectl apply -f cluster/deck_service.yaml

.PHONY: deck-image deck-deployment deck-service

splice-image: bazel-release-push

splice-deployment: get-cluster-credentials
	kubectl apply -f cluster/splice_deployment.yaml

.PHONY: tot-image splice-image splice-deployment

tot-image: bazel-release-push

tot-deployment: get-cluster-credentials
	kubectl apply -f cluster/tot_deployment.yaml

tot-service: get-cluster-credentials
	kubectl apply -f cluster/tot_service.yaml

.PHONY: tot-image tot-deployment

horologium-image: bazel-release-push

horologium-deployment: get-cluster-credentials
	kubectl apply -f cluster/horologium_deployment.yaml

.PHONY: horologium-image horologium-deployment

plank-image: bazel-release-push

plank-deployment: get-cluster-credentials
	kubectl apply -f cluster/plank_deployment.yaml

.PHONY: plank-image plank-deployment

jenkins-operator-image: bazel-release-push

jenkins-operator-deployment: get-cluster-credentials
	kubectl apply -f cluster/jenkins_deployment.yaml

pushgateway-deploy: get-cluster-credentials
	kubectl apply -f cluster/pushgateway_deployment.yaml

.PHONY: jenkins-operator-image jenkins-operator-deployment pushgateway-deploy

tide-image: bazel-release-push

tide-deployment: get-cluster-credentials
	kubectl apply -f cluster/tide_deployment.yaml

mem-range-deployment: get-build-cluster-credentials
	kubectl apply -f cluster/mem_limit_range.yaml

.PHONY: tide-image tide-deployment mem-range-deployment

clonerefs-image: bazel-release-push

initupload-image: bazel-release-push
gcsupload-image: bazel-release-push
entrypoint-image: bazel-release-push
sidecar-image: bazel-release-push

.PHONY: clonerefs-image initupload-image gcsupload-image entrypoint-image sidecar-image

needs-rebase-image: bazel-release-push

needs-rebase-deployment: get-cluster-credentials
	kubectl apply -f cluster/needs-rebase_deployment.yaml

needs-rebase-service: get-cluster-credentials
	kubectl apply -f cluster/needs-rebase_service.yaml

.PHONY: needs-rebase-image needs-rebase-deployment needs-rebase-service

checkconfig-image: bazel-release-push
crier-image: bazel-release-push

.PHONY: crier-image checkconfig-image
