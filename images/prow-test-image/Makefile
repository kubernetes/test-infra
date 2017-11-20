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

IMG = gcr.io/k8s-testimages/prow-test-image
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# <kubernetes version>:<golang version>
VERSIONS := 1.6:1.7.4 1.7:1.8.3 1.8:1.8.3 1.9:1.9.1


do_build = \
	$(eval VERSION_PARTS := $(subst :, ,$(v))) \
	$(eval K8S := $(firstword $(VERSION_PARTS))) \
	$(eval GO := $(lastword $(VERSION_PARTS))) \
	docker build --no-cache --build-arg GO_VERSION=$(GO) -f ./Dockerfile -t $(IMG):$(TAG)-$(K8S) . &&\
	docker tag $(IMG):$(TAG)-$(K8S) $(IMG):latest-$(K8S) &&\
	echo Built $(IMG):$(TAG)-$(K8S) and tagged with $(IMG):latest-$(K8S) ;

do_push = \
    $(eval VERSION_PARTS := $(subst :, ,$(v))) \
    $(eval K8S := $(firstword $(VERSION_PARTS))) \
	gcloud docker -- push $(IMG):$(TAG)-$(K8S);\
	gcloud docker -- push $(IMG):latest-$(K8S);\
	echo Pushed $(IMG):latest-$(K8S) and $(IMG):$(TAG)-$(K8S);

.PHONY: all build push help

all: build

build:
	@$(foreach v,$(VERSIONS),$(do_build))

push: build
	@$(foreach v,$(VERSIONS),$(do_push))

help:
	@echo "TARGETS:"
	@echo " all:            build"
	@echo " build:      	builds all of the VERSIONS"
	@echo " push:       	builds all the VERSIONS and pushes to k8s-testimages"

