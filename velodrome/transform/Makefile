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

# Please set IMG
TAG := $(shell date +v%Y%m%d-%H%M%S)
IMG ?= k8s.gcr.io/github-transform

all: push

build:
	CGO_ENABLED=0 go build .
	docker build -t $(IMG):$(TAG) .
	@echo Built $(IMG):$(TAG)

push: build
	docker push $(IMG):$(TAG)
	@echo Pushed $(IMG):$(TAG)
