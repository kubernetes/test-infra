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

IMG = gcr.io/k8s-testimages/kubekins-test
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)
VERSIONS := $(wildcard ./Dockerfile-*)

do_build = \
	$(eval VERSION := $(subst ./Dockerfile-,,$(v)))\
	docker build -f ./Dockerfile-$(VERSION) -t $(IMG):$(VERSION)-$(TAG) . ;\
	docker tag $(IMG):$(VERSION)-$(TAG) $(IMG):$(VERSION)-latest ;\
	echo Built $(IMG):$(VERSION)-$(TAG) and tagged with $(IMG):$(VERSION)-latest ;

do_push = \
	$(eval VERSION := $(subst ./Dockerfile-,,$(v)))\
	gcloud docker -- push $(IMG):$(VERSION)-$(TAG);\
	gcloud docker -- push $(IMG):$(VERSION)-latest;\
	echo Pushed $(IMG):$(VERSION)-latest and $(IMG):$(VERSION)-$(TAG);

.PHONY: all build push help

all: build

build:
	@$(foreach v,$(VERSIONS),$(do_build))

push: build
	@$(foreach v,$(VERSIONS),$(do_push))

help:
	@echo "VARIABLES:"
	@echo " VERSIONS:       Dockerfile to be built/pushed, can be either full file name or a tag"
	@echo " 				- make build VERSIONS=./Dockerfile-1.4"
	@echo " 				- make build VERSIONS=1.4"
	@echo " 				are equivalent and both valid."
	@echo ""
	@echo "TARGETS:"
	@echo " all:            build"
	@echo " build:      	builds all the Dockerfile-* in VERSIONS"
	@echo " push:       	builds all the Dockerfile-* in VERSIONS and pushes to k8s-testimages"

