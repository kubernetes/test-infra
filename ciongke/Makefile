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

# These are the usual GKE variables.
PROJECT = k8s-ci-on-gke
ZONE = us-central1-f
NUM_NODES = 3
MACHINE_TYPE = n1-standard-2

# These are GitHub credentials in files on your own machine.
# The hook secret is your HMAC token, the OAuth secret is the OAuth
# token of whatever account you want to comment and update statuses.
HOOK_SECRET_FILE = ${HOME}/hook
OAUTH_SECRET_FILE = ${HOME}/k8s-oauth-token

# The Jenkins secret is the API token, and the address file contains Jenkins'
# URL, such as http://pull-jenkins-master:8080, without a newline.
JENKINS_SECRET_FILE = ${HOME}/jenkins
JENKINS_ADDRESS_FILE = ${HOME}/jenkins-address

# Useful rules:
# - create-cluster turns up a cluster then prints out the webhook address.
# - update-cluster pushes new image versions and updates the deployment.
# - hook-image builds and pushes the hook image.
# - hook-deployment updates the deployment.
# - hook-service create the hook service.
# - test-pr-image builds and pushes the test-pr image.

create-cluster:
	gcloud -q container --project "$(PROJECT)" clusters create ciongke --zone "$(ZONE)" --machine-type "$(MACHINE_TYPE)" --scope "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --num-nodes "$(NUM_NODES)" --network "default" --enable-cloud-logging --enable-cloud-monitoring
	@kubectl create secret generic hmac-token --from-file=hmac=$(HOOK_SECRET_FILE)
	@kubectl create secret generic oauth-token --from-file=oauth=$(OAUTH_SECRET_FILE)
	@kubectl create secret generic jenkins-token --from-file=jenkins=$(JENKINS_SECRET_FILE)
	@kubectl create configmap jenkins-address --from-file=jenkins-address=$(JENKINS_ADDRESS_FILE)
	@make test-pr-image --no-print-directory
	@make hook-image --no-print-directory
	@make hook-deployment --no-print-directory
	@make hook-service --no-print-directory
	@echo -n "Waiting for loadbalancer ingress "; while [[ "$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip)" == "null" ]]; do echo -n "."; sleep 5; done; echo " Done"
	@echo "Webhook address: http://$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip):8888/"
	@gcloud compute --project "$(PROJECT)" addresses create ciongke --addresses "$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip)"
	@echo "Temporary hotfix until 1.3.7: If the cluster is not in the Jenkins project, add a firewall rule from the cluster nodes to the Jenkins master. Set the cluster node IPs to static."

update-cluster:
	make test-pr-image --no-print-directory
	make hook-image --no-print-directory
	make hook-deployment --no-print-directory

clean:
	rm cmd/hook/hook cmd/test-pr/test-pr

test:
	go test -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: create-cluster update-cluster clean test

hook-image:
	CGO_ENABLED=0 go build -o cmd/hook/hook github.com/kubernetes/test-infra/ciongke/cmd/hook
	docker build -t "gcr.io/k8s-ci-on-gke/hook:0.14" cmd/hook
	gcloud docker push "gcr.io/k8s-ci-on-gke/hook:0.14"

hook-deployment:
	kubectl apply -f hook_deployment.yaml

hook-service:
	kubectl create -f hook_service.yaml

test-pr-image:
	CGO_ENABLED=0 go build -o cmd/test-pr/test-pr github.com/kubernetes/test-infra/ciongke/cmd/test-pr
	docker build -t "gcr.io/k8s-ci-on-gke/test-pr:0.7" cmd/test-pr
	gcloud docker push "gcr.io/k8s-ci-on-gke/test-pr:0.7"

.PHONY: hook-image hook-deployment hook-service test-pr-image
