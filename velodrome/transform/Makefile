# Please set IMG
TAG := $(shell date +v%Y%m%d-%H%M%S)
IMG ?= gcr.io/google-containers/github-transform

all: push

build:
	CGO_ENABLED=0 go build .
	docker build -t $(IMG):$(TAG) .
	@echo Built $(IMG):$(TAG)

push: build
	gcloud docker push $(IMG):$(TAG)
	@echo Pushed $(IMG):$(TAG)
