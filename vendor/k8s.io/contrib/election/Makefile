all: push

# 0.0 shouldn't clobber any released builds
TAG = 0.0
PREFIX = gcr.io/google_containers/leader-elector

server:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w' -o server example/main.go

container: server
	docker build -t $(PREFIX):$(TAG) .

push: container
	gcloud docker push $(PREFIX):$(TAG)

clean:
	rm -f server
