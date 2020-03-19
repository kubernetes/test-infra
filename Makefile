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

# Image URL to use all building/pushing image targets;
# Use your own docker registry and image name for dev/test by overridding the
# IMAGE_REPO, *_IMAGE_NAME environment variable.
IMAGE_REPO ?= quay.io/multicloudlab
CHECK_TOOL_IMAGE_NAME ?= check-tool
BUILD_TOOL_IMAGE_NAME ?= build-tool

GIT_HOST = github.com/IBM
PWD := $(shell pwd)
BASE_DIR := $(shell basename $(PWD))

# Keep an existing GOPATH, make a private one if it is undefined
GOPATH_DEFAULT := $(PWD)/.go
export GOPATH ?= $(GOPATH_DEFAULT)
GOBIN_DEFAULT := $(GOPATH)/bin
export GOBIN ?= $(GOBIN_DEFAULT)
TESTARGS_DEFAULT := "-v"
export TESTARGS ?= $(TESTARGS_DEFAULT)
DEST := $(GOPATH)/src/$(GIT_HOST)/$(BASE_DIR)
VERSION ?= $(shell date +v%Y%m%d)-$(shell git describe --match=$(git rev-parse --short=8 HEAD) --tags --always --dirty)

# Image URL to use all building/pushing image targets
# IMG ?= xxx
# REGISTRY ?= quay.io/multicloudlab

ifneq ("$(realpath $(DEST))", "$(realpath $(PWD))")
	$(error Please run 'make' from $(DEST). Current directory is $(PWD))
endif

include common/Makefile.common.mk

all: check test build

############################################################
# work section
############################################################
$(GOBIN):
	@echo "create gobin"
	@mkdir -p $(GOBIN)

work: $(GOBIN)

############################################################
# check section
############################################################
check: fmt
#check: fmt lint

fmt: format-go format-protos format-python

lint: lint-all

############################################################
# test section
############################################################

test:
#	@go test ${TESTARGS} ./...

############################################################
# build section
############################################################

build:
#	@go build ./...

############################################################
# images section
############################################################

image: check-tool-image build-tool-image build-tool-multiarch-image

check-tool-image: config-docker
	@IMAGE_REPO=$(IMAGE_REPO) CHECK_TOOL_IMAGE_NAME=$(CHECK_TOOL_IMAGE_NAME) VERSION=$(VERSION) \
	cd prow/docker/check-tool && ./build-and-push.sh

build-tool-image: config-docker
	@IMAGE_REPO=$(IMAGE_REPO) BUILD_TOOL_IMAGE_NAME=$(BUILD_TOOL_IMAGE_NAME) VERSION=$(VERSION) \
	cd prow/docker/build-tool && ./build-and-push.sh

build-tool-multiarch-image: config-docker
	@MAX_PULLING_RETRY=20 RETRY_INTERVAL=120 common/scripts/multiarch_image.sh $(IMAGE_REPO) $(BUILD_TOOL_IMAGE_NAME) $(VERSION)

############################################################
# clean section
############################################################
clean:
	@rm -f xxx
