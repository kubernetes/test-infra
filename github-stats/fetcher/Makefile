# Please set IMG
TAG = $(shell date +v%Y%m%d)
IMG ?= gcr.io/google-containers/fetcher

all: push

build:
	CGO_ENABLED=0 go build .
	docker build -t $(IMG):$(TAG) .
	docker tag -f $(IMG):$(TAG) $(IMG):latest
	@echo Built $(IMG):$(TAG) and tagged with latest

push: build
	gcloud docker push $(IMG):$(TAG)
	gcloud docker push $(IMG):latest
	@echo Pushed $(IMG) with :latest and :$(TAG) tags
