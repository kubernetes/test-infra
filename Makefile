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
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o mungegithub

# build the container with the binary
container: mungegithub
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
deploy: push rc
	# Get existing RCs
	$(eval rcs := $(shell kubectl --kubeconfig=$(KUBECONFIG) get rc --selector='app=$(APP),readonly=$(READONLY)' --output=template --template='{{range .items}}{{.metadata.name}} {{end}}'))
	# Deploy the new RC
	kubectl --kubeconfig=$(KUBECONFIG) create -f $(APP)/local.rc.yaml
	# Scale the old RCs to 0
	$(foreach rc,$(rcs),kubectl --kubeconfig=$(KUBECONFIG) scale --replicas=0 rc $(rc);)
	# Current RCs, you might want to clean some up!
	kubectl --kubeconfig=$(KUBECONFIG) get rc

# Removes all mungegithub RCs
cleanrcs:
	$(eval rcs := $(shell kubectl --kubeconfig=$(KUBECONFIG) get rc --selector='app=$(APP),readonly=$(READONLY)' --output=template --template='{{range .items}}{{.metadata.name}} {{end}}'))
	$(foreach rc,$(rcs),kubectl --kubeconfig=$(KUBECONFIG) delete rc $(rc);)

# Try to run the binary locally using docker, doesn't need to push or have a running kube cluster.
# Binary is exposed on port 8080
local_dryrun: container
	docker run --rm -v $(TOKEN):/token -p 8080:8080 $(CONTAINER)

# updates the rc.yaml with current build information and sets it to --dry-run
rc:
	# update the rc.yaml with the current date and git hash
	sed -e 's|[[:digit:]]\{4\}-[[:digit:]]\{2\}-[[:digit:]]\{2\}-[[:xdigit:]]\+|$(TAG)|g' $(APP)/rc.yaml > $(APP)/local.rc.yaml
	# update the rc.yaml with the current repo (if not gcr.io
	sed -i -e 's|gcr.io/google_containers|$(REPO)|g' $(APP)/local.rc.yaml
ifeq ($(READONLY),false)
	# update the rc.yaml with --dry-run=false
	sed -i -e 's!^\([[:space:]]\+\)- --dry-run=true!\1- --dry-run=false!g' $(APP)/local.rc.yaml
endif
	# update the rc.yaml with label "readonly: true"
	sed -i -e 's!^\([[:space:]]\+\)app: $(APP)!\1app: $(APP)\n\1readonly: "$(READONLY)"!g' $(APP)/local.rc.yaml

# simple transformation of a github oauth secret file to a kubernetes secret
secret:
	@echo $(token)
	sed -e 's|1234567890123456789012345678901234567890123456789012345=|$(token)|' $(APP)/secret.yaml > $(APP)/local.secret.yaml

clean:
	rm -f mungegithub $(APP)/local.rc.yaml $(APP)/local.secret.yaml

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
	@echo " deploy:       launches the container on a kubernetes cluster, it will scale all other RCs to 0, but not delete them"
	@echo " cleanrcs:     removes all RCs deployed READONLY=$(READONLY)"
	@echo " local_dryrun: tries to launch the container locally with docker"
	@echo " rc:           updates $(APP)/rc.yaml and places results in $(APP)/local.rc.yaml"
	@echo " secret:       updates $(APP)/secret.yaml with TOKEN an creates $(APP)/local.secret.yaml"
	@echo " clean:        deletes the binary and local files (does not delete old containers)"


.PHONY: all mungegithub container push dryrun cleandryrun local_dryrun rc secret clean help
