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

all: build fmt vet test


HOOK_VERSION   = 0.50
LINE_VERSION   = 0.26
SINKER_VERSION = 0.3
DECK_VERSION   = 0.4

# These are the usual GKE variables.
PROJECT = kubernetes-jenkins-pull
ZONE = us-central1-f

# These are GitHub credentials in files on your own machine.
# The hook secret is your HMAC token, the OAuth secret is the OAuth
# token of whatever account you want to comment and update statuses.
HOOK_SECRET_FILE = ${HOME}/hook
OAUTH_SECRET_FILE = ${HOME}/k8s-oauth-token

# The Jenkins secret is the API token, and the address file contains Jenkins'
# URL, such as http://pull-jenkins-master:8080, without a newline.
JENKINS_SECRET_FILE = ${HOME}/jenkins
JENKINS_ADDRESS_FILE = ${HOME}/jenkins-address

# Service account key for bootstrap jobs.
SERVICE_ACCOUNT_FILE = ${HOME}/service-account.json

# Useful rules:
# - create-cluster turns up a cluster then prints out the webhook address.
# - update-cluster pushes new image versions and updates the deployment.
# - update-jobs updates the configmap defining the Jenkins jobs.
# - hook-image builds and pushes the hook image.
# - hook-deployment updates the deployment.
# - hook-service create the hook service.
# - line-image builds and pushes the line image.
# - sinker-image builds and pushes the sinker image.
# - sinker-deployment updates the sinker deployment.
# - update-godeps deletes and recreates vendored dependencies.

# Should probably move this to a script or something.
create-cluster:
	gcloud -q container --project "$(PROJECT)" clusters create ciongke --zone "$(ZONE)" --machine-type n1-standard-4 --num-nodes 4 --node-labels=role=ciongke --scopes "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --network "default" --enable-cloud-logging --enable-cloud-monitoring
	gcloud -q container node-pools create build-pool --project "$(PROJECT)" --cluster "ciongke" --zone "$(ZONE)" --machine-type n1-standard-8 --num-nodes 4 --local-ssd-count=1 --node-labels=role=build
	kubectl create secret generic hmac-token --from-file=hmac=$(HOOK_SECRET_FILE)
	kubectl create secret generic oauth-token --from-file=oauth=$(OAUTH_SECRET_FILE)
	kubectl create secret generic jenkins-token --from-file=jenkins=$(JENKINS_SECRET_FILE)
	kubectl create secret generic service-account --from-file=service-account.json=$(JENKINS_SECRET_FILE)
	kubectl create configmap jenkins-address --from-file=jenkins-address=$(JENKINS_ADDRESS_FILE)
	kubectl create configmap job-configs --from-file=jobs=jobs.yaml
	kubectl create configmap plugins --from-file=plugins=plugins.yaml
	@make line-image --no-print-directory
	@make hook-image --no-print-directory
	@make deck-image --no-print-directory
	@make sinker-image --no-print-directory
	@make hook-deployment --no-print-directory
	@make hook-service --no-print-directory
	@make sinker-deployment --no-print-directory
	@make deck-deployment --no-print-directory
	@make deck-service --no-print-directory
	@echo -n "Waiting for hook ingress IP "; while [[ "$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip)" == "null" ]]; do echo -n "."; sleep 5; done; echo " Done"
	@echo "Webhook address: http://$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip):8888/"
	gcloud compute --project "$(PROJECT)" addresses create ciongke --addresses "$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip)"
	@echo -n "Waiting for deck ingress IP "; while [[ "$$(kubectl get svc deck -o=json | jq -r .status.loadBalancer.ingress[0].ip)" == "null" ]]; do echo -n "."; sleep 5; done; echo " Done"
	@echo "Deck address: http://$$(kubectl get svc deck -o=json | jq -r .status.loadBalancer.ingress[0].ip)/"
	gcloud compute --project "$(PROJECT)" addresses create deck --addresses "$$(kubectl get svc deck -o=json | jq -r .status.loadBalancer.ingress[0].ip)"

update-cluster: get-cluster-credentials
	@make line-image --no-print-directory
	@make hook-image --no-print-directory
	@make sinker-image --no-print-directory
	@make deck-image --no-print-directory
	@make hook-deployment --no-print-directory
	@make sinker-deployment --no-print-directory
	@make deck-deployment --no-print-directory

update-jobs: get-cluster-credentials
	kubectl create configmap job-configs --from-file=jobs=jobs.yaml --dry-run -o yaml | kubectl replace configmap job-configs -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

get-cluster-credentials:
	gcloud container clusters get-credentials ciongke --project="$(PROJECT)" --zone="$(ZONE)"

clean:
	rm cmd/hook/hook cmd/line/line cmd/sinker/sinker cmd/deck/deck

build:
	go install cmd/...

fmt:
	(diff=$$(find . -name "*.go" | grep -v "\/vendor\/" | xargs gofmt -d -s 2>&1); if [[ -n "$${diff}" ]]; then echo "$${diff}"; false; fi)

vet:
	go vet $$(go list ./... | grep -v "\/vendor\/")

test:
	go test -race -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: create-cluster update-cluster update-jobs update-plugins clean build fmt vet test get-cluster-credentials

hook-image:
	CGO_ENABLED=0 go build -o cmd/hook/hook k8s.io/test-infra/prow/cmd/hook
	docker build -t "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)" cmd/hook
	gcloud docker -- push "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)"

hook-deployment:
	kubectl apply -f cluster/hook_deployment.yaml

hook-service:
	kubectl create -f cluster/hook_service.yaml

line-image:
	CGO_ENABLED=0 go build -o cmd/line/line k8s.io/test-infra/prow/cmd/line
	docker build -t "gcr.io/$(PROJECT)/line:$(LINE_VERSION)" cmd/line
	gcloud docker -- push "gcr.io/$(PROJECT)/line:$(LINE_VERSION)"

sinker-image:
	CGO_ENABLED=0 go build -o cmd/sinker/sinker k8s.io/test-infra/prow/cmd/sinker
	docker build -t "gcr.io/$(PROJECT)/sinker:$(SINKER_VERSION)" cmd/sinker
	gcloud docker -- push "gcr.io/$(PROJECT)/sinker:$(SINKER_VERSION)"

sinker-deployment:
	kubectl apply -f cluster/sinker_deployment.yaml

deck-image:
	CGO_ENABLED=0 go build -o cmd/deck/deck k8s.io/test-infra/prow/cmd/deck
	docker build -t "gcr.io/$(PROJECT)/deck:$(DECK_VERSION)" cmd/deck
	gcloud docker -- push "gcr.io/$(PROJECT)/deck:$(DECK_VERSION)"

deck-deployment:
	kubectl apply -f cluster/deck_deployment.yaml

deck-service:
	kubectl create -f cluster/deck_service.yaml

.PHONY: hook-image hook-deployment hook-service test-pr-image sinker-image sinker-deployment deck-image deck-deployment deck-service

update-godeps:
	rm -rf vendor Godeps
	godep save ./...

.PHONY: update-godeps
