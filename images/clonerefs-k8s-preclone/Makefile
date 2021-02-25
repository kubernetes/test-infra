# TODO: replace this makefile with using the cloudbuild setup

# get current clonerefs image from prow
CLONEREFS_IMAGE=$(shell go run ./get_default_clonerefs_image.go)
TAG=$(shell sed -En 's/.*:(v.*)/\1/p' <<<'$(CLONEREFS_IMAGE)')

# TODO replace this
REGISTRY=gcr.io/spiffxp-gke-dev
IMAGE=$(REGISTRY)/clonerefs-k8s-preclone:$(TAG)

# TODO: issues pushing to GCR with buildkit
build:
	DOCKER_BUILDKIT=0 docker build -t $(IMAGE) .

push: build
	DOCKER_BUILDKIT=0 docker push $(IMAGE)