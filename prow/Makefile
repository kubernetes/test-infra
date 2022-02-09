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

################################################################################
# ========================= Setup Go With Gimme ================================
# go version to use for build etc.
# setup correct go version with gimme
PATH:=$(shell . ../hack/build/setup-go.sh && echo "$${PATH}")
# go1.9+ can autodetect GOROOT, but if some other tool sets it ...
GOROOT:=
# enable modules
GO111MODULE=on
export PATH GOROOT GO111MODULE
# work around broken PATH export
SPACE:=$(subst ,, )
SHELL:=env PATH=$(subst $(SPACE),\$(SPACE),$(PATH)) $(SHELL)
################################################################################

.PHONY: push-images
push-images:
	go run ../hack/imagebuilder --ko-docker-repo="${REGISTRY}" --push
