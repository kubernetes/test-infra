all: container

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

DEFAULTREPO := gcr.io/google_containers
REPO ?= $(DEFAULTREPO)
APP ?= submit-queue
CONTAINER := $(REPO)/$(APP):$(TAG)

KUBECONFIG ?= $(HOME)/.kube/config

TARGET ?= kubernetes

TOKEN ?= "./token"
token=$(shell cat $(TOKEN))
token64=$(shell base64 $(TOKEN))

READONLY ?= true

# just build the binary
mungegithub build:
	GOBIN=$(PWD) CGO_ENABLED=0 GOOS=linux go install -installsuffix cgo -ldflags '-w'

test: mungegithub
	# in case of error ignore all lines of 'getsockopt: connection refused' these are background go threads that don't matter
	CGO_ENABLED=0 GOOS=linux go test $(shell go list ./... | grep -v 'vendor/') | grep -v 'getsockopt: connection refused'

# build the container with the binary
container: mungegithub
	docker build --pull -t $(CONTAINER) -f Dockerfile-$(APP) .

# push the container
push: container
ifneq (,$(findstring gcr.io,$(REPO)))
	gcloud docker -- push $(CONTAINER)
else
	docker push $(CONTAINER)
endif

# Launch the container on a cluster (with --dry-run).
# The cluster will likely need a service to get access to the web interface (see service.yaml)
# The cluster will need a github oauth token (the secret target makes that easy to create)
deploy: push deployment
	# Deploy the new deployment
	kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/local.deployment.yaml --record

# A new configuration is pushed by using the configmap specified using $(TARGET) and $(APP).
push_config:
	# pushes a new configmap.
	kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/deployment/$(TARGET)/configmap.yaml

# updates the deployment.yaml with current build information and sets it to --dry-run
deployment:
	# update the deployment.yaml with the current date and git hash
	sed -e 's|[[:digit:]]\{4\}-[[:digit:]]\{2\}-[[:digit:]]\{2\}-[[:xdigit:]]\+|$(TAG)|g' $(APP)/deployment.yaml > $(APP)/local.deployment.yaml
	# update the deployment.yaml with the current repo (if not gcr.io
	sed -i -e 's|gcr.io/google_containers|$(REPO)|g' $(APP)/local.deployment.yaml
ifeq ($(READONLY),false)
	# update the deployment.yaml with --dry-run=false
	sed -i -e 's!^\([[:space:]]\+\)- --dry-run=true!\1- --dry-run=false!g' $(APP)/local.deployment.yaml
endif
	# update the deployment.yaml with label "readonly: true"
	sed -i -e 's!^\([[:space:]]\+\)app: $(APP)!\1app: $(APP)\n\1readonly: "$(READONLY)"!g' $(APP)/local.deployment.yaml

	# String-replacement of @@ by $(TARGET) in the deployment file.
	sed -i -e 's!@@!$(TARGET)!g' $(APP)/local.deployment.yaml

# simple transformation of a github oauth secret file to a kubernetes secret
secret:
	@echo $(token64)
	sed -e 's|1234567890123456789012345678901234567890123456789012345=|$(token64)|' $(APP)/deployment/$(TARGET)/secret.yaml > $(APP)/local.secret.yaml

clean:
	rm -f mungegithub $(APP)/local.deployment.yaml $(APP)/local.secret.yaml

# pull down current public queue state, and run UI based off that data
ui-stub:
	@/bin/bash -c "wget -q -r -nH -P ./submit-queue/www http://submit-queue.k8s.io/{prs,github-e2e-queue,history,sq-stats,stats,users,health,google-internal-ci,priority-info,merge-info}; \
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
	@echo " all:          runs 'container'"
	@echo " mungegithub:  builds the binary"
	@echo " container:    builds the binary and creates a container with the binary"
	@echo " push:         pushes the container to the registry"
	@echo " deploy:       launches/updates the app on a kubernetes cluster"
	@echo " push_config:  applys changes to configMaps for the app/target. $(APP)/deployment/$(TARGET)/configmap.yaml"
	@echo " deployment:   updates $(APP)/deployment.yaml and places results in $(APP)/local.deployment.yaml"
	@echo " ui-stub:      grab upstream submit-queue data and launch the web ui on port 8000"
	@echo " secret:       updates $(APP)/secret.yaml with TOKEN an creates $(APP)/local.secret.yaml"
	@echo " clean:        deletes the binary and local files (does not delete old containers)"


.PHONY: all mungegithub test container push dryrun cleandryrun deployment secret clean help
