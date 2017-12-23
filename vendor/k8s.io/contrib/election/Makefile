all: push

# 0.0 shouldn't clobber any released builds
# current latest is 0.5
TAG = 0.0
PREFIX = gcr.io/google_containers/leader-elector

NODEJS_TAG = 0.1
NODEJS_PREFIX = gcr.io/google_containers/nodejs-election-client

server:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w' -o server example/main.go

container: server
	docker build -t $(PREFIX):$(TAG) .
	docker build -t $(NODEJS_PREFIX):$(NODEJS_TAG) client/nodejs

push: container
	gcloud docker push $(PREFIX):$(TAG)
	gcloud docker push $(NODEJS_PREFIX):$(NODEJS_TAG)

clean:
	rm -f server
