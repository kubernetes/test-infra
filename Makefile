all: container

DATE := $(shell date +%F)
GIT := $(shell git rev-parse --short HEAD)

TAG ?= $(DATE)-$(GIT)

mungegithub:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o mungegithub

update_pod_version:
	sed -i -e 's|[[:digit:]]\{4\}-[[:digit:]]\{2\}-[[:digit:]]\{2\}-[[:xdigit:]]\+|$(TAG)|g' pod.yaml

container: mungegithub update_pod_version
	docker build -t gcr.io/google_containers/mungegithub:$(TAG) .

push: container
	gcloud docker push gcr.io/google_containers/mungegithub:$(TAG)

clean:
	rm -f mungegithub
