# Please set IMG
TAG = $(shell date +v%Y%m%d)

build:
	docker build -t $(IMG):$(TAG) .
	docker tag $(IMG):$(TAG) $(IMG):latest
	@echo Built $(IMG):$(TAG) and tagged with latest

push:
	gcloud docker push $(IMG):$(TAG)
	gcloud docker push $(IMG):latest
	@echo Pushed $(IMG) with :latest and :$(TAG) tags

all: build push
