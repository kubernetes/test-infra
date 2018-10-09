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

PROJECT = k8s-testimages
IMG = gcr.io/$(PROJECT)/logexporter
TAG = v0.1.3

.PHONY: build push

all: build

cmd/logexporter: cmd/main.go
	CGO_ENABLED=0 go build -o cmd/logexporter k8s.io/test-infra/logexporter/cmd

build: cmd/logexporter
	docker build --pull -t $(IMG):$(TAG) cmd
	docker tag $(IMG):$(TAG) $(IMG):latest
	rm cmd/logexporter
	@echo Built $(IMG):$(TAG) and tagged with latest

push: build
	docker push $(IMG):$(TAG)
	docker push $(IMG):latest
	@echo Pushed $(IMG) with :latest and :$(TAG) tags
