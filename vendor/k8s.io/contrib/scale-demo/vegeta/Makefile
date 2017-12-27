all: push

# See pod.yaml for the version currently running-- bump this ahead before rebuilding!
TAG = 0.6

loader: loader.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 godep go build -a -installsuffix cgo -ldflags '-w' -o loader

container: loader
	docker build -t gcr.io/google_containers/loader:$(TAG) .

push: container
	gcloud docker push gcr.io/google_containers/loader:$(TAG)

clean:
	rm -f loader
