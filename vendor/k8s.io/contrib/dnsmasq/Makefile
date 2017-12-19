# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# Makefile for the Docker image gcr.io/google_containers/kube-dnsmasq-ARCH
# MAINTAINER: Abhishek Shah <abshah@google.com>
# If you update this image please bump the tag value before pushing.
#
# Usage:
#   [TAG=1.3] [REGISTRY=gcr.io/google_containers] make (build|push)

# Default registry, arch and tag. This can be overwritten by arguments to make
TAG?=1.3
REGISTRY?=gcr.io/google_containers
ARCH?=amd64
KUBE_CROSS_IMAGE:=$(REGISTRY)/kube-cross:v1.6.2-2
DNSMASQ_VERSION?=dnsmasq-2.76
TEMP_DIR:=$(shell mktemp -d)

ifeq ($(ARCH),arm)
	BASEIMAGE?=armel/busybox:glibc
    TRIPLE?=arm-linux-gnueabi
    QEMUARCH=arm
endif
ifeq ($(ARCH),arm64)
	BASEIMAGE?=aarch64/busybox:glibc
    TRIPLE?=aarch64-linux-gnu
    QEMUARCH=aarch64
endif
ifeq ($(ARCH),ppc64le)
	BASEIMAGE?=ppc64le/busybox:glibc
    TRIPLE?=powerpc64le-linux-gnu
    QEMUARCH=ppc64le
endif

all: build

build:
ifeq ($(ARCH),amd64)
	cp Dockerfile $(TEMP_DIR)
	# When building "normally" for amd64, remove the whole line, it has no part in the amd64 image
	cd $(TEMP_DIR) && sed -i "/CROSS_BUILD_/d" Dockerfile
else
	cp Dockerfile.cross $(TEMP_DIR)/Dockerfile
	cd $(TEMP_DIR) && sed -i "s|BASEIMAGE|$(BASEIMAGE)|g" Dockerfile
	cd $(TEMP_DIR) && sed -i "s|ARCH|$(QEMUARCH)|g" Dockerfile

	# Build the dnsmasq binary using the cross-compilation docker image
	docker run -it --rm -v $(TEMP_DIR):/build $(KUBE_CROSS_IMAGE) /bin/bash -c \
	"curl -sSL  http://www.thekelleys.org.uk/dnsmasq/$(DNSMASQ_VERSION).tar.xz | tar -xJ \
	&& cd $(DNSMASQ_VERSION) \
	&& CC=$(TRIPLE)-gcc make \
	&& cp src/dnsmasq /build"

	# When cross-building, only the placeholder "CROSS_BUILD_" should be removed
	# Register /usr/bin/qemu-ARCH-static as the handler for ARM binaries in the kernel
	docker run --rm --privileged multiarch/qemu-user-static:register --reset
	curl -sSL https://github.com/multiarch/qemu-user-static/releases/download/v2.5.0/x86_64_qemu-$(QEMUARCH)-static.tar.xz | tar -xJ -C $(TEMP_DIR)
	cd $(TEMP_DIR) && sed -i "s/CROSS_BUILD_//g" Dockerfile
endif
	cp dnsmasq.conf $(TEMP_DIR)
	docker build -t $(REGISTRY)/kube-dnsmasq-$(ARCH):$(TAG) $(TEMP_DIR)
	rm -rf $(TEMP_DIR)

push: build
ifeq ($(REGISTRY),gcr.io/google_containers)
	gcloud docker push $(REGISTRY)/kube-dnsmasq-$(ARCH):$(TAG)
else
	docker push $(REGISTRY)/kube-dnsmasq-$(ARCH):$(TAG)
endif

.PHONY: push
