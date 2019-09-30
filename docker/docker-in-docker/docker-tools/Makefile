# Copyright 2019 The Kubernetes Authors.
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

CONTAINER_CLI ?= docker
DOCKER_REGISTRY ?= quay.io
DOCKER_USERNAME ?= multicloudlab
IMAGE ?= multicloudlab-builder
VERSION ?= $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

image:
	$(CONTAINER_CLI) build --cache-from $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(IMAGE):latest \
		-t $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(IMAGE):$(VERSION) \
		-t $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(IMAGE):latest -f Dockerfile ../

push:
	$(CONTAINER_CLI) push $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(IMAGE):latest
	$(CONTAINER_CLI) push $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(IMAGE):$(VERSION)

.PHONY: image push
