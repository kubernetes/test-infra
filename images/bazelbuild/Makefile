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

IMG = gcr.io/k8s-testimages/bazelbuild
TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# TODO: retire some of these as they become unnecessary
LATEST_BAZEL_VERSION=0.10.0
BAZEL_VERSIONS=0.6.1 0.7.0 0.8.1 $(LATEST_BAZEL_VERSION)

all: build

do_build = \
	docker build --pull --build-arg BAZEL_VERSION=$(VERSION) --build-arg IMAGE_ARG=$(IMG):$(TAG)-$(VERSION) -t $(IMG):$(TAG)-$(VERSION) .;\
	docker tag $(IMG):$(TAG)-$(VERSION) $(IMG):latest-$(VERSION);\
	echo Built $(IMG):$(TAG)-$(VERSION) and tagged with latest-$(VERSION);\
	[ "$(VERSION)" == "$(LATEST_BAZEL_VERSION)" ] &&\
	docker tag $(IMG):$(TAG)-$(VERSION) $(IMG):latest-latest &&\
	echo tagged $(IMG) with latest-latest tag;

do_push = \
	docker push $(IMG):$(TAG)-$(VERSION);\
	docker push $(IMG):latest-$(VERSION);\
	echo Pushed $(IMG) with $(TAG)-$(VERSION) and latest-$(VERSION) tags;\
	[ "$(VERSION)" == "$(LATEST_BAZEL_VERSION)" ] &&\
	docker push $(IMG):latest-latest &&\
	echo Pushed $(IMG) with latest-latest tag;

build:
	@$(foreach VERSION,$(BAZEL_VERSIONS),$(do_build))

push: build
	@$(foreach VERSION,$(BAZEL_VERSIONS),$(do_push))

.PHONY: all build push
