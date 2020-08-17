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

K8S ?= everything
EXTRA_ARG =

ifneq ($(K8S), everything)
  EXTRA_ARG = --variant $(K8S)
endif

push-prod:
	bazel run //images/builder -- $(EXTRA_ARG) --project=k8s-testimages --scratch-bucket=gs://k8s-testimages-scratch --build-dir=. images/kubekins-e2e

push:
	bazel run //images/builder -- $(EXTRA_ARG) --allow-dirty --build-dir=. images/kubekins-e2e

.PHONY: push push-prod help

help:
	@echo "By default, build all the images"
	@echo "But you can also specify one of a supported k8s version, \"experimental\", or \"master\", e.g."
	@echo
	@echo "    make build K8S=1.14"
	@echo
