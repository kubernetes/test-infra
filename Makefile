all: container

TAG = 0.9.alpha

mungegithub:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o mungegithub

update_pod_version:
	sed -i -e 's|[0-9]\+\.[0-9]\+\(\.[^"]\+\)\?|$(TAG)|g' pod.yaml

container: mungegithub update_pod_version
	docker build -t gcr.io/google_containers/mungegithub:$(TAG) .

push: container
	gcloud docker push gcr.io/google_containers/mungegithub:$(TAG)

clean:
	rm -f mungegithub
