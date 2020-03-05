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

PROGRAM ?= aws-janitor-boskos
HUB ?= gcr.io/k8s-testimages
IMAGE ?= $(HUB)/$(notdir $(PROGRAM))
TAG ?= $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

build: $(PROGRAM)
$(PROGRAM):
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o "$@"

image:
	GOOS=linux GOARCH=amd64 $(MAKE)
	docker build $(DOCKER_OPTS) -t "$(IMAGE):$(TAG)" .

push: image
	docker push "$(IMAGE):$(TAG)"

push-latest: push
	docker tag "$(IMAGE):$(TAG)" "$(IMAGE):latest"
	docker push "$(IMAGE):latest"

clean:
	rm -f $(PROGRAM)

.PHONY: $(PROGRAM) build clean image push push-latest
