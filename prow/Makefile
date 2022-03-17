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
REGISTRY ?= gcr.io/k8s-prow
PROW_IMAGE ?=

.PHONY: push-images
push-images:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="${REGISTRY}" --push=true

.PHONY: build-images
build-images:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="ko.local" --push=false

.PHONY: push-single-image
push-single-image:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="${REGISTRY}" --push=true --image=${PROW_IMAGE}

.PHONY: build-single-image
build-single-image:
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="ko.local" --push=false --image=${PROW_IMAGE}

.PHONY: build-tarball
build-tarball:
# use --ko-docker-repo="something.not.exist" as ko skips writing `.tar` file if
# it's `ko.local.
	../hack/make-rules/go-run/arbitrary.sh run ./hack/prowimagebuilder --ko-docker-repo="something.not.exist" --push=false --image=${PROW_IMAGE}
