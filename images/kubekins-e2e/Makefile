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

IMG = gcr.io/k8s-testimages/kubekins-e2e
TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# kubernetes master branch config
K8S ?= master
GO ?= 1.11.2
BAZEL ?= 0.18.1
UPGRADE_DOCKER ?=false
# TODO(bentheelder,cblecker): this is aging and bad
CFSSL ?= R1.2

# config for testing prior to rolling out to master / ongoing release
ifeq ($(K8S), experimental)
	GO = 1.11.2
	BAZEL = 0.20.0
	CFSSL = R1.2
	UPGRADE_DOCKER = true
endif

# kubernetes release-X branch configs
ifeq ($(K8S), 1.13)
	GO = 1.11.2
	BAZEL = 0.18.1
	CFSSL = R1.2
endif

ifeq ($(K8S), 1.12)
	GO = 1.10.4
	BAZEL = 0.16.1
	CFSSL = R1.2
endif

ifeq ($(K8S), 1.11)
	GO = 1.10.2
	BAZEL = 0.14.0
	CFSSL = R1.2
endif

ifeq ($(K8S), 1.10)
	GO = 1.9.3
	BAZEL = 0.14.0
	CFSSL = R1.2
endif


all: build

kubetest:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o kubetest ../../kubetest

build: kubetest
	@echo Building with go:$(GO) and bazel:$(BAZEL)
	docker build --build-arg GO_VERSION=$(GO) --build-arg BAZEL_VERSION_ARG=$(BAZEL) --build-arg CFSSL_VERSION=$(CFSSL) --build-arg IMAGE_ARG=$(IMG):$(TAG)-$(K8S) --build-arg=UPGRADE_DOCKER_ARG=$(UPGRADE_DOCKER) -t $(IMG):$(TAG)-$(K8S) .
	docker tag $(IMG):$(TAG)-$(K8S) $(IMG):latest-$(K8S)
	rm kubetest
	@echo Built $(IMG):$(TAG)-$(K8S) and tagged with latest-$(K8S)

push: build
	docker push $(IMG):$(TAG)-$(K8S)
	docker push $(IMG):latest-$(K8S)
	@echo Pushed $(IMG) with :latest-$(K8S) and :$(TAG)-$(K8S) tags

.PHONY: all kubetest build push help

help:
	@echo "By default, build a master image"
	@echo "But you can also specify a k8s version, i.e. :"
	@echo "    - K8S=1.8 make build"
