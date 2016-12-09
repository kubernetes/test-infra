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


HOOK_VERSION   = 0.64
LINE_VERSION   = 0.36
SINKER_VERSION = 0.4
DECK_VERSION   = 0.9
SPLICE_VERSION = 0.7
MARQUE_VERSION = 0.1

# These are the usual GKE variables.
PROJECT = k8s-prow
ZONE = us-central1-f
CLUSTER = prow

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

# Should probably move this to a script or something.
create-cluster:
	gcloud -q container --project "$(PROJECT)" clusters create "$(CLUSTER)" --zone "$(ZONE)" --machine-type n1-standard-4 --num-nodes 4 --node-labels=role=prow --scopes "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --network "default" --enable-cloud-logging --enable-cloud-monitoring
	gcloud -q container node-pools create build-pool --project "$(PROJECT)" --cluster "$(CLUSTER)" --zone "$(ZONE)" --machine-type n1-standard-8 --num-nodes 4 --local-ssd-count=1 --node-labels=role=build
	kubectl create secret generic hmac-token --from-file=hmac=$(HOOK_SECRET_FILE)
	kubectl create secret generic oauth-token --from-file=oauth=$(OAUTH_SECRET_FILE)
	kubectl create secret generic jenkins-token --from-file=jenkins=$(JENKINS_SECRET_FILE)
	kubectl create secret generic service-account --from-file=service-account.json=$(SERVICE_ACCOUNT_FILE)
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
	@make splice-image --no-print-directory
	@make splice-deployment --no-print-directory
	kubectl apply -f cluster/ingress.yaml

update-cluster: get-cluster-credentials
	@make line-image --no-print-directory
	@make hook-image --no-print-directory
	@make sinker-image --no-print-directory
	@make deck-image --no-print-directory
	@make hook-deployment --no-print-directory
	@make sinker-deployment --no-print-directory
	@make deck-deployment --no-print-directory
	@make splice-image --no-print-directory
	@make splice-deployment --no-print-directory

update-jobs: get-cluster-credentials
	kubectl create configmap job-configs --from-file=jobs=jobs.yaml --dry-run -o yaml | kubectl replace configmap job-configs -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

clean:
	rm cmd/hook/hook cmd/line/line cmd/sinker/sinker cmd/deck/deck cmd/splice/splice

build:
	go install ./cmd/...

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

splice-image:
	CGO_ENABLED=0 go build -o cmd/splice/splice k8s.io/test-infra/prow/cmd/splice
	docker build -t "gcr.io/$(PROJECT)/splice:$(SPLICE_VERSION)" cmd/splice
	gcloud docker -- push "gcr.io/$(PROJECT)/splice:$(SPLICE_VERSION)"

splice-deployment:
	kubectl apply -f cluster/splice_deployment.yaml

marque-image:
	CGO_ENABLED=0 go build -o cmd/marque/marque k8s.io/test-infra/prow/cmd/marque
	docker build -t "gcr.io/$(PROJECT)/marque:$(MARQUE_VERSION)" cmd/marque
	gcloud docker -- push "gcr.io/$(PROJECT)/marque:$(MARQUE_VERSION)"

marque-deployment:
	kubectl apply -f cluster/marque_deployment.yaml

marque-service:
	kubectl apply -f cluster/marque_service.yaml

.PHONY: hook-image hook-deployment hook-service test-pr-image sinker-image sinker-deployment deck-image deck-deployment deck-service splice-image splice-deployment marque-image marque-deployment marque-service

update-godeps:
	rm -rf vendor Godeps
	godep save ./...

.PHONY: update-godeps
