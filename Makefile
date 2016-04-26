all: container

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

DEFAULTREPO := gcr.io/google_containers
REPO ?= $(DEFAULTREPO)
APP ?= submit-queue
CONTAINER := $(REPO)/$(APP):$(TAG)

KUBECONFIG ?= $(HOME)/.kube/config

TOKEN ?= "./token"
token=$(shell base64 $(TOKEN))

READONLY ?= true

# just build the binary
mungegithub:
	CGO_ENABLED=0 GOOS=linux GO15VENDOREXPERIMENT=0 godep go build -a -installsuffix cgo -ldflags '-w' -o mungegithub

test: mungegithub
	CGO_ENABLED=0 GOOS=linux GO15VENDOREXPERIMENT=0 godep go test ./...

# build the container with the binary
container: test
	docker build -t $(CONTAINER) -f Dockerfile-$(APP) .

# push the container
push: container
ifeq ($(REPO),$(DEFAULTREPO))
	gcloud docker push $(CONTAINER)
else
	docker push $(CONTAINER)
endif

# Launch the container on a cluster (with --dry-run).
# The cluster will likely need a service to get access to the web interface (see service.yaml)
# The cluster will need a github oauth token (the secret target makes that easy to create)
deploy: push deployment
	# Deploy the new deployment
	kubectl --kubeconfig=$(KUBECONFIG) apply -f $(APP)/local.deployment.yaml --record

# Try to run the binary locally using docker, doesn't need to push or have a running kube cluster.
# Binary is exposed on port 8080
local_dryrun: container
	docker run --rm -v $(TOKEN):/token -p 8080:8080 $(CONTAINER)

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

# simple transformation of a github oauth secret file to a kubernetes secret
secret:
	@echo $(token)
	sed -e 's|1234567890123456789012345678901234567890123456789012345=|$(token)|' $(APP)/secret.yaml > $(APP)/local.secret.yaml

clean:
	rm -f mungegithub $(APP)/local.deployment.yaml $(APP)/local.secret.yaml

help:
	@echo "ENVIRONMENT VARS:"
	@echo " REPO=       repository for the docker image being build. Default: $(REPO)"
	@echo " TOKEN=      file with github oauth token, needed in local_dryrun and secret. Default: $(TOKEN)"
	@echo " KUBECONFIG= kubeconfig file for deployment. Default: $(KUBECONFIG)"
	@echo " READONLY=   should the container actually mute github objects or just do everything else. Default: $(READONLY)"
	@echo " APP=        which application you are trying to deploy. cherrypick or submit-queue. Default: $(APP)"
	@echo ""
	@echo "TARGETS:"
	@echo " all:          runs 'container'"
	@echo " mungegithub:  builds the binary"
	@echo " container:    builds the binary and creates a container with the binary"
	@echo " push:         pushes the container to the registry"
	@echo " deploy:       launches/updates the app on a kubernetes cluster"
	@echo " local_dryrun: tries to launch the container locally with docker"
	@echo " deployment:   updates $(APP)/deployment.yaml and places results in $(APP)/local.deployment.yaml"
	@echo " secret:       updates $(APP)/secret.yaml with TOKEN an creates $(APP)/local.secret.yaml"
	@echo " clean:        deletes the binary and local files (does not delete old containers)"


.PHONY: all mungegithub test container push dryrun cleandryrun local_dryrun deployment secret clean help
