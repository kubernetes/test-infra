all: push

# See pod.yaml for the version currently running-- bump this ahead before rebuilding!
TAG = 0.11

PROJ = google_containers

.PHONY: perfdash
perfdash:
	cd .. && bazel build perfdash
	cp ../bazel-bin/perfdash/linux_amd64_stripped/perfdash .

container: perfdash
	docker build --pull -t gcr.io/$(PROJ)/perfdash:$(TAG) .

push: container
	gcloud docker -- push gcr.io/$(PROJ)/perfdash:$(TAG)

clean:
	rm -f perfdash
