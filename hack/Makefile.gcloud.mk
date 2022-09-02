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

# # README
#
# These rules are for setting up Kube current-context, so that later kubectl command
# knows which cluster to work on.
# It is highly recommended to import this rule for manipulating prow/build clusters,
# which helps prevent from targeting current-cluster context from your local environment.

# For working with prow service cluster:
#	CLUSTER = "<CLUSTER_NAME>" # e.g. `prow`
#	PROJECT = "<GCP_PROJECT_NAME_WHERE_CLUSTER_IS_IN>" # e.g. `k8s-prow`
#	ZONE = "<GCP_ZONE_NAME_WHERE_CLUSTER_IS_IN>" # e.g. us-west1-b
#	deploy-prow: get-cluster-credentials
#
# For working with prow build cluster:
#	CLUSTER_BUILD = "<BUILD_CLUSTER_NAME>" # e.g. `prow-build`
#	PROJECT_BUILD = "<GCP_PROJECT_NAME_WHERE_BUILD_CLUSTER_IS_IN>" # e.g. `k8s-prow-builds`
#	ZONE_BUILD = "<GCP_ZONE_NAME_WHERE_BUILD_CLUSTER_IS_IN >" # e.g. us-west1-b
#	deploy-prow: get-build-cluster-credentials

# These rules only take effect on rule explicitly importing them, it won't be applied on subsequent
# kubectl calls, nor changes your local Kube current-context.

export KUBECONFIG

# By default get-cluster-credentials saves Kube current-context after execution.
# Add this rule to prevents the Kube current-context in the execution environment from being
# overwritten unless the intention is made explicit w/ the `save` parameter.
# e.g.
#		make get-cluster-credentials save=true
.PHONY: save-kubeconfig
save-kubeconfig:
ifndef save
	$(eval KUBECONFIG=$(shell mktemp))
endif

.PHONY: activate-serviceaccount
activate-serviceaccount:
ifdef GOOGLE_APPLICATION_CREDENTIALS
	gcloud auth activate-service-account --key-file="$(GOOGLE_APPLICATION_CREDENTIALS)"
endif

.PHONY: get-cluster-credentials
get-cluster-credentials: save-kubeconfig activate-serviceaccount
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

.PHONY: get-build-cluster-credentials
get-build-cluster-credentials: save-kubeconfig activate-serviceaccount
	gcloud container clusters get-credentials "$(CLUSTER_BUILD)" --project="$(PROJECT_BUILD)" --zone="$(ZONE_BUILD)"
