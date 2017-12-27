all: push

# See pod.yaml for the version currently running-- bump this ahead before rebuilding!
TAG = v0.2

REPO = gcr.io/google_containers

node-perf-dash: node-perf-dash.go parser.go downloader.go data.go kubelet-parser.go
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o node-perf-dash

container: node-perf-dash
	docker build -t $(REPO)/node-perf-dash:$(TAG) .

push: container
	gcloud docker push $(REPO)/node-perf-dash:$(TAG)

clean:
	rm -f node-perf-dash
