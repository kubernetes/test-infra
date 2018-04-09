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

all: bazel


# TODO(fejta): deploy via bazel
# These are the usual GKE variables.
PROJECT       ?= k8s-prow
ZONE          ?= us-central1-f
CLUSTER       ?= prow

bazel:
	@echo Use bazel foo or ./planter/planter.sh bazel foo
	@echo "  bazel build //label_sync/...  # build"
	@echo "  bazel test //label_sync/...  # test"
	@echo "  bazel run //label_sync:push  # push image"
	@echo "  bazel run //label_sync:deploy # deploy"
	@exit 1

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

update-config: get-cluster-credentials
	# Oddy the first time requires creating it instead of replacing it...
	kubectl create configmap label-config --from-file=labels.yaml=labels.yaml --dry-run -o yaml | kubectl replace configmap label-config -f -

label_sync-job: get-cluster-credentials
	kubectl apply -f cluster/label_sync_job.yaml

label_sync-cronjob: get-cluster-credentials
	kubectl apply -f cluster/label_sync_cron_job.yaml

.PHONY: label_sync-job label_sync-cronjob bazel get-cluster-credentials
