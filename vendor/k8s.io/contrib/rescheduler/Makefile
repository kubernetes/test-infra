all: build

FLAGS =
ENVVAR = GOOS=linux GOARCH=amd64 CGO_ENABLED=0
REGISTRY = gcr.io/google_containers
TAG = v0.2.1

deps:
	go get github.com/tools/godep

build: clean deps
	$(ENVVAR) godep go build ./...
	$(ENVVAR) godep go build -o rescheduler 

test-unit: clean deps build
	$(ENVVAR) godep go test --test.short -race ./... $(FLAGS)

container: build
	docker build -t ${REGISTRY}/rescheduler:$(TAG) .

push: container
	gcloud docker push ${REGISTRY}/rescheduler:$(TAG)

clean:
	rm -f rescheduler

.PHONY: all deps build test-unit container push clean
