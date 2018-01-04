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
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

K8S ?= master

GO ?= 1.9.2
BAZEL ?= 0.8.1
CFSSL ?= R1.2

ifeq ($(K8S), 1.9)
	GO = 1.9.2
	BAZEL = 0.8.1
	CFSSL = R1.2
endif

ifeq ($(K8S), 1.8)
	GO = 1.8.3
	BAZEL = 0.6.1
	CFSSL = R1.2
endif

ifeq ($(K8S), 1.7)
	GO = 1.8.3
	BAZEL = 0.6.1
	CFSSL = R1.2
endif

# TODO(krzyzacy): deprecate 1.6 images
ifeq ($(K8S), 1.6)
	GO = 1.7.4
	BAZEL = 0.5.2
	CFSSL = R1.2
endif

all: build

kubetest:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o kubetest ../../kubetest

build: kubetest
	@echo Building with go:$(GO) and bazel:$(BAZEL)
	docker build --build-arg GO_VERSION=$(GO) --build-arg BAZEL_VERSION=$(BAZEL) --build-arg CFSSL_VERSION=$(CFSSL) --build-arg IMAGE_ARG=$(IMG):$(TAG)-$(K8S) -t $(IMG):$(TAG)-$(K8S) .
	docker tag $(IMG):$(TAG)-$(K8S) $(IMG):latest-$(K8S)
	rm kubetest
	@echo Built $(IMG):$(TAG)-$(K8S) and tagged with latest-$(K8S)

push: build
	gcloud docker -- push $(IMG):$(TAG)-$(K8S)
	gcloud docker -- push $(IMG):latest-$(K8S)
	@echo Pushed $(IMG) with :latest-$(K8S) and :$(TAG)-$(K8S) tags

.PHONY: all kubetest build push help

help:
	@echo "By default, build a master image"
	@echo "But you can also specify a k8s version, i.e. :"
	@echo "    - K8S=1.8 make build"
