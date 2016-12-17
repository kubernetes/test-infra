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

IMG = gcr.io/k8s-testimages/kubekins-node
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# default to 1.6 image
VERSION:=1.6
KUBEKINS_TAG:=1.6-v20161205-ad918bc

# Resolve VERSION
ifeq ($(VERSION),1.4)
	KUBEKINS_TAG=1.4-v20161130-8958f82
endif
ifeq ($(VERSION),1.5)
	KUBEKINS_TAG=1.5-v20161205-d664d14
endif

all: build

build:
	sed 's/__KUBEKINS_TAG/$(KUBEKINS_TAG)/g' Dockerfile > Dockerfile-$(VERSION)

	docker build -f ./Dockerfile-$(VERSION) -t $(IMG):$(VERSION)-$(TAG) .
	docker tag $(IMG):$(VERSION)-$(TAG) $(IMG):$(VERSION)-latest
	@echo Built $(IMG):$(VERSION)-$(TAG) and tagged with $(VERSION)-latest

	rm Dockerfile-$(VERSION)

push: build
	gcloud docker -- push $(IMG):$(VERSION)-$(TAG)
	gcloud docker -- push $(IMG):$(VERSION)-latest
	@echo Pushed $(IMG) with :$(VERSION)-latest and :$(VERSION)-$(TAG) tags

help:
	@echo "VARIABLES:"
	@echo " VERSIONS:       Kubekins Version Reference, default is 1.6"
	@echo " 				exp: make build VERSIONS=1.5 will build a 1.5 node image"
	@echo ""
	@echo "TARGETS:"
	@echo " build:      	builds kubekins-node image with VERSION, or 1.6 by default"
	@echo " push:       	builds kubekins-node image with VERSION and pushes to k8s-testimages"
