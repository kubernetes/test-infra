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

KUBEKINS_IMG = gcr.io/k8s-testimages/kubekins-e2e
KUBEKINS_BAZEL_IMG = gcr.io/k8s-testimages/kubekins-e2e-bazel
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

all: build

kubetest:
	GOOS=linux GOARCH=amd64 go build -o kubetest ../../kubetest

build: kubetest
	docker build -t $(KUBEKINS_IMG):$(TAG) -f Dockerfile-kubekins-e2e .
	docker tag $(KUBEKINS_IMG):$(TAG) $(KUBEKINS_IMG):latest
	docker build -t $(KUBEKINS_BAZEL_IMG):$(TAG) -f Dockerfile-kubekins-e2e-bazel .
	docker tag $(KUBEKINS_BAZEL_IMG):$(TAG) $(KUBEKINS_BAZEL_IMG):latest
	rm kubetest
	@echo Built $(KUBEKINS_BAZEL_IMG):$(TAG) and $(KUBEKINS_BAZEL_IMG):$(TAG) and tagged with latest

push: build
	gcloud docker -- push $(KUBEKINS_IMG):$(TAG)
	gcloud docker -- push $(KUBEKINS_IMG):latest
	gcloud docker -- push $(KUBEKINS_BAZEL_IMG):$(TAG)
	gcloud docker -- push $(KUBEKINS_BAZEL_IMG):latest
	@echo Pushed $(KUBEKINS_IMG) and $(KUBEKINS_BAZEL_IMG) with :latest and :$(TAG) tags
