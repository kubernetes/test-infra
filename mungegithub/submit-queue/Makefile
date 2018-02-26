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

KUBECONFIG ?= $(HOME)/.kube/config

push-nginx:
	# Replace the nginx config and restart the pod.
	kubectl --kubeconfig=$(KUBECONFIG) delete configmap nginx-https-redirect && \
	kubectl --kubeconfig=$(KUBECONFIG) create configmap nginx-https-redirect --from-file=nginx.conf=nginx-redirect/nginx.conf && \
	kubectl --kubeconfig=$(KUBECONFIG) delete pod -l app=nginx-https-redirect

push-ingress:
	# Update the ingress.
	kubectl --kubeconfig=$(KUBECONFIG) apply --record -f ingress.yaml

# Updates nginx https proxy and the ingress with the local nginx-redirect/nginx.conf and ingress.yaml files.
push-network: push-nginx push-ingress

targets := $(shell ls -d deployment/*/ | sed -e "s|deployment/\(.*\)/|\1|"| sed -e "s|^$$||")

update-all:
	@echo "Updating submit queues for repo(s) [$(targets)] in 4 seconds..."
	@sleep 4
	@$(foreach repo,$(targets),echo "Updating $(repo) submit-queue:"; APP=submit-queue TARGET=$(repo) make --no-print-directory -f ../Makefile push_config deploy;)

deploy-all:
	@echo "Deploying submit queues for repo(s) [$(targets)] in 4 seconds..."
	@sleep 4
	@$(foreach repo,$(targets),echo "Deploying $(repo) submit-queue:"; APP=submit-queue TARGET=$(repo) make --no-print-directory -f ../Makefile first_time_deploy;)

.PHONY: push-network update-all deploy-all push-nginx push-ingress
