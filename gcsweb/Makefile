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

# The binary to build (just the basename).
BIN := gcsweb

# This repo's root import path (under GOPATH).
PKG := k8s.io/test-infra/gcsweb

# Where to push the docker image.
REGISTRY ?= staging-k8s.gcr.io

# Which architecture to build - see $(ALL_ARCH) for options.
ARCH ?= amd64

# The version string.
VERSION := v1.1.0

###
### These variables should not need tweaking.
###

SRC_DIRS := cmd pkg # directories which hold app source (not vendored)

ALL_ARCH := amd64 arm arm64 ppc64le s390x

IMAGE := $(REGISTRY)/$(BIN)

BUILD_IMAGE ?= golang:1.11.5

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# If you want to build all binaries, see the 'all-build' rule.
# If you want to build all containers, see the 'all-container' rule.
# If you want to build AND push all containers, see the 'all-push' rule.
all: build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

container-%:
	@$(MAKE) --no-print-directory ARCH=$* container

push-%:
	@$(MAKE) --no-print-directory ARCH=$* push

all-build: $(addprefix build-, $(ALL_ARCH))

all-container: $(addprefix container-, $(ALL_ARCH))

all-push-images: $(addprefix push-, $(ALL_ARCH))

build: bin/$(ARCH)/$(BIN)

bin/$(ARCH)/$(BIN): build-dirs
	@echo "building: $@"
	@docker run                                                            \
	    -ti                                                                \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/bin/$(ARCH):/go/bin/$$(go env GOOS)_$(ARCH)             \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ARCH=$(ARCH)                                                   \
	        VERSION=$(VERSION)                                             \
	        PKG=$(PKG)                                                     \
	        ./build/build.sh                                               \
	    "

DOTFILE_IMAGE = $(subst /,_,$(IMAGE))-$(ARCH)-$(VERSION)

container: .container-$(DOTFILE_IMAGE) container-name
.container-$(DOTFILE_IMAGE): bin/$(ARCH)/$(BIN) Dockerfile.in
	@sed \
	    -e 's|ARG_BIN|$(BIN)|g' \
	    -e 's|ARG_ARCH|$(ARCH)|g' \
	    Dockerfile.in > .dockerfile-$(ARCH)
	@chmod -R go+r icons styles
	@docker build --pull -t $(IMAGE)-$(ARCH):$(VERSION) -f .dockerfile-$(ARCH) .
	@docker images -q $(IMAGE)-$(ARCH):$(VERSION) > $@

container-name:
	@echo "container: $(IMAGE)-$(ARCH):$(VERSION)"

push: .push-$(DOTFILE_IMAGE) push-name
.push-$(DOTFILE_IMAGE): .container-$(DOTFILE_IMAGE)
	@docker push $(IMAGE)-$(ARCH):$(VERSION)
	@docker images -q $(IMAGE)-$(ARCH):$(VERSION) > $@

push-name:
	@echo "pushed: $(IMAGE)-$(ARCH):$(VERSION)"

version:
	@echo $(VERSION)

test: build-dirs
	@docker run                                                            \
	    -ti                                                                \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ./build/test.sh $(SRC_DIRS)                                    \
	    "

build-dirs:
	@mkdir -p bin/$(ARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(ARCH)

clean: container-clean bin-clean

container-clean:
	rm -rf .container-* .dockerfile-* .push-*

bin-clean:
	rm -rf .go bin

all-push: all-push-images push-manifest

push-manifest:
	docker manifest create --amend $(IMAGE):$(VERSION) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(IMAGE)\-&:$(VERSION)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} $(IMAGE):$(VERSION) $(IMAGE)-$${arch}:$(VERSION); done
	docker manifest push --purge $(IMAGE):$(VERSION)
