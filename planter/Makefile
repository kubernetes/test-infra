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

# note: sync this with planter.sh!
# this should be bazel version - planter sub version
BAZEL_VERSION ?= 0.22.0
IMAGE_NAME ?= gcr.io/k8s-testimages/planter
TAG = $(BAZEL_VERSION)

image:
	docker build --build-arg BAZEL_VERSION=$(BAZEL_VERSION) -t "$(IMAGE_NAME):$(TAG)" . --pull
	docker build --build-arg BASEIMAGE="$(IMAGE_NAME):$(TAG)" -t "$(IMAGE_NAME):$(TAG)-cross" -f Dockerfile-cross .

push: image
	docker push "$(IMAGE_NAME):$(TAG)"
	docker push "$(IMAGE_NAME):$(TAG)-cross"

.PHONY: image push
