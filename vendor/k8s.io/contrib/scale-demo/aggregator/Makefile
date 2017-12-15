all: container

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

aggregator:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o aggregator

container: aggregator
	docker build -t gcr.io/google_containers/aggregator:$(TAG) .

local_dryrun: container
	docker run --rm -p 8090:8080 gcr.io/google_containers/aggregator:$(TAG)

push: container
	gcloud docker push gcr.io/google_containers/aggregator:$(TAG)

clean:
	rm -f aggregator 

.PHONY: all aggregator update_pod_version container push clean local_dryrun
