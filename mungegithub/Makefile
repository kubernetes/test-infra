# Copyright 2015 The Kubernetes Authors.
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

all: container

# Path Prefix: the path of the mungegithub directory.
DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
CD := cd $(DIR) &&

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

DEFAULTREPO := gcr.io/k8s-testimages
REPO ?= $(DEFAULTREPO)
APP ?= submit-queue
CONTAINER := $(REPO)/$(APP):$(TAG)

KUBECONFIG ?= $(HOME)/.kube/config

TARGET ?= kubernetes

TOKEN ?= "./token"
token64:=$(shell base64 $(TOKEN))

HOOKSECRET ?= "./hook-secret"
hooksecret64:=$(shell base64 $(HOOKSECRET))

READONLY ?= true

CLUSTER ?= mungegithub
PROJECT ?= k8s-mungegithub
ZONE ?= us-central1-b

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

# just build the binary
mungegithub build:
	$(CD) GOBIN=$(DIR) CGO_ENABLED=0 GOOS=linux go install -installsuffix cgo -ldflags '-w'

test: mungegithub
	# in case of error ignore all lines of 'getsockopt: connection refused' these are background go threads that don't matter
	$(CD) CGO_ENABLED=0 GOOS=linux go test $(shell go list ./... | grep -v 'vendor/') | grep -v 'getsockopt: connection refused'

# build the container with the binary
container: mungegithub
	$(CD) docker build --pull -t $(CONTAINER) -f Dockerfile-$(APP) .

# push the container
push: container
ifneq (,$(findstring gcr.io,$(REPO)))
	docker push $(CONTAINER)
else
	docker push $(CONTAINER)
endif

# Launch the container on a cluster (with --dry-run).
# The cluster will likely need a service to get access to the web interface (see service.yaml)
# The cluster will need a github oauth token (the secret target makes that easy to create)
deploy: push deployment get-cluster-credentials
	# Deploy the new deployment
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/local.deployment.yaml --record

# A new configuration is pushed by using the configmap specified using $(TARGET) and $(APP).
push_config: get-cluster-credentials
	# pushes a new configmap.
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) create configmap $(TARGET)-$(APP)-config --from-file=$(APP)/deployment/$(TARGET) --dry-run -o yaml | kubectl apply -f -

# Creates a service resource spec for the specified target and app.
service:
ifneq ("", "$(wildcard $(APP)/service.yaml)")
	# updating service.yaml by replacing @@ with $(TARGET).
	$(CD) sed -e 's!@@!$(TARGET)!g' $(APP)/service.yaml > $(APP)/local.service.yaml
endif

# Pushes a service resource spec.
push_service: service get-cluster-credentials
ifneq ("", "$(wildcard $(APP)/service.yaml)")
	# Applying service.yaml.
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/local.service.yaml
endif

# updates the deployment.yaml with current build information and sets it to --dry-run
deployment:
	# update the deployment.yaml with the current date and git hash
	$(CD) sed -e 's|[[:digit:]]\{4\}-[[:digit:]]\{2\}-[[:digit:]]\{2\}-[[:xdigit:]]\+|$(TAG)|g' $(APP)/deployment.yaml > $(APP)/local.deployment.yaml
	# update the deployment.yaml with the current repo (if not gcr.io
	$(CD) sed -i -e 's|gcr.io/k8s-testimages|$(REPO)|g' $(APP)/local.deployment.yaml
ifeq ($(READONLY),false)
	# update the deployment.yaml with --dry-run=false
	$(CD) sed -i -e 's!^\([[:space:]]\+\)- --dry-run=true!\1- --dry-run=false!g' $(APP)/local.deployment.yaml
endif
	# update the deployment.yaml with label "readonly: true"
	$(CD) sed -i -e 's!^\([[:space:]]\+\)app: $(APP)!\1app: $(APP)\n\1readonly: "$(READONLY)"!g' $(APP)/local.deployment.yaml
	# String-replacement of @@ by $(TARGET) in the deployment file.
	$(CD) sed -i -e 's!@@!$(TARGET)!g' $(APP)/local.deployment.yaml

# simple transformation of a github oauth secret file to a kubernetes secret
secret:
	# Updating secret.yaml and outputing to $(APP)/local.secret.yaml.
	$(CD) sed -e 's|<TOKEN>|$(token64)|' $(APP)/secret.yaml > $(APP)/local.secret.yaml
	$(CD) sed -i -e 's|<HOOKSECRET>|$(hooksecret64)|' $(APP)/local.secret.yaml
	# String-replacement of @@ by $(TARGET) in the secrets file.
	$(CD) sed -i -e 's!@@!$(TARGET)!g' $(APP)/local.secret.yaml

# Generate and deploy the token secret.
push_secret: secret get-cluster-credentials
	# Appling local.secret.yaml.
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/local.secret.yaml

# Generate resource spec files for the persistent volume and claim.
volume:
ifneq ("", "$(and $(wildcard $(APP)/pv.yaml), $(wildcard $(APP)/pvc.yaml))")
	# Updating pv.yaml and pvc.yaml by replacing @@ with TARGET.
	$(CD) sed -e 's!@@!$(TARGET)!g' $(APP)/pv.yaml > $(APP)/local.pv.yaml
	$(CD) sed -e 's!@@!$(TARGET)!g' $(APP)/pvc.yaml > $(APP)/local.pvc.yaml
endif

# Create the persistent volume and persistent volume claim.
push_volume: volume get-cluster-credentials
ifneq ("", "$(and $(wildcard $(APP)/pv.yaml), $(wildcard $(APP)/pvc.yaml))")
	# Creating persistent volume and persistent volume claim.
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) create -f $(APP)/local.pv.yaml
	# Sleeping for 1s to allow persistent volume to be created.
	@sleep 1
	$(CD) kubectl --kubeconfig=$(KUBECONFIG) create -f $(APP)/local.pvc.yaml
	# Sleeping for 4s to allow claim to bind to volume.
	@sleep 4
endif

# Deploys after completing first time setup such as creating the volume and volume claim, and creating the token secret, and service.
first_time_deploy: push_volume push_secret push_service push_config deploy

clean:
	$(CD) rm -f mungegithub $(APP)/local.deployment.yaml $(APP)/local.secret.yaml

# pull down current public queue state, and run UI based off that data
ui-stub:
	@$(CD) /bin/bash -c "wget -q -r -nH -P ./submit-queue/www http://submit-queue.k8s.io/{prs,github-e2e-queue,history,sq-stats,stats,users,health,google-internal-ci,priority-info,merge-info}; \
	pushd ./submit-queue/www; \
	python -m SimpleHTTPServer;"

help:
	@echo "ENVIRONMENT VARS:"
	@echo " REPO=       repository for the docker image being build. Default: $(REPO)"
	@echo " TOKEN=      file with github oauth token, needed in secret. Default: $(TOKEN)"
	@echo " KUBECONFIG= kubeconfig file for deployment. Default: $(KUBECONFIG)"
	@echo " READONLY=   should the container actually mute github objects or just do everything else. Default: $(READONLY)"
	@echo " APP=        which application you are trying to deploy. cherrypick or submit-queue. Default: $(APP)"
	@echo " TARGET=     which repository this should run against. ex: contrib, test-infra, kubernetes. Default: $(TARGET)"
	@echo ""
	@echo "TARGETS:"
	@echo " all:              runs 'container'"
	@echo " mungegithub:      builds the binary"
	@echo " container:        builds the binary and creates a container with the binary"
	@echo " push:             pushes the container to the registry"
	@echo " deploy:           launches/updates the app on a kubernetes cluster"
	@echo " push_config:      applys changes to configMaps for the app/target. $(APP)/deployment/$(TARGET)/configmap.yaml"
	@echo " deployment:       updates $(APP)/deployment.yaml and places results in $(APP)/local.deployment.yaml"
	@echo " ui-stub:          grab upstream submit-queue data and launch the web ui on port 8000"
	@echo " secret:           updates $(APP)/secret.yaml with TOKEN and HOOKSECRET and creates $(APP)/local.secret.yaml"
	@echo " push_secret:      creates $(APP)/local.secret.yaml using the secret make target and applies it to the cluster."
	@echo " service:           updates $(APP)/service.yaml with TARGET and creates $(APP)/local.service.yaml"
	@echo " push_service:      creates $(APP)/local.service.yaml using the service make target and applies it to the cluster."
	@echo " clean:            deletes the binary and local files (does not delete old containers)"


.PHONY: all mungegithub test container push push_config service push_service deployment secret push_secret volume push_volume first_time_deploy clean ui-stub help
