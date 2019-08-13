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

EXTRA_ARG =
VERSION ?= all
ifneq ($(VERSION), all)
	EXTRA_ARG = --variant $(VERSION)
endif

push-prod:
  bazel run //images/builder -- $(EXTRA_ARG) --project=k8s-testimages --scratch-bucket=gs://k8s-testimages-scratch images/kubekins-test

push:
	bazel run //images/builder -- $(EXTRA_ARG) --allow-dirty images/kubekins-test

.PHONY: all build push help

help:
	@echo "VARIABLES:"
	@echo " VERSION: Version to be pushed. If omitted, all images are pushed."
	@echo "   - make push VERSION=1.4"
	@echo ""
	@echo "TARGETS:"
	@echo " push:       	builds all the images and pushes to your current project's GCR"
	@echo " push-prod:    builds all the images and pushes to k8s-testimages"
