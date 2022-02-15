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

# Build and push specific variables.
# Production prow images are still built and pushed from bazel rules. Building
# and pushing images with this Make rule to staging registry, will migrate once
# https://github.com/kubernetes/test-infra/issues/23832 is done
REGISTRY ?= gcr.io/k8s-prow-staging
PROW_IMAGES ?=

.PHONY: push-images
push-images:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="${REGISTRY}" --push=true

.PHONY: build-images
build-images:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="ko.local" --push=false
