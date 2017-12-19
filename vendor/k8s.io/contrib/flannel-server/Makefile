all: push

# 0.0 shouldn't clobber any released builds
TAG = 0.0
PREFIX = gcr.io/google_containers/flannel-server-helper

server: main.go
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o flannel_helper ./main.go

container: server
	docker build -t $(PREFIX):$(TAG) .

push: container
	gcloud docker push $(PREFIX):$(TAG)

clean:
	rm -f flannel_helper
