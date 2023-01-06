# Copyright 2021 The Kubernetes Authors.
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

include ../../../../hack/Makefile.gcloud.mk

PROJECT       ?= k8s-prow
CLUSTER       ?= prow
ZONE          ?= us-central1-f

apply-components: get-cluster-credentials
	kubectl apply -f ./prometheus-operator-crds/
	kubectl apply -f .

apply: apply-components
	$(MAKE) -C ./mixins apply

.PHONY: apply-components apply
