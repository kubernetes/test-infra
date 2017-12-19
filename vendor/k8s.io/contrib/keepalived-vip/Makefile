all: push

# 0.0 shouldn't clobber any release builds, current "latest" is 0.6
TAG = 0.6
PREFIX = gcr.io/google_containers/kube-keepalived-vip
BUILD_IMAGE = build-keepalived

controller: clean
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w' -o kube-keepalived-vip

container: controller keepalived
	docker build -t $(PREFIX):$(TAG) .

keepalived:
	docker build -t $(BUILD_IMAGE):$(TAG) build
	docker create --name $(BUILD_IMAGE) $(BUILD_IMAGE):$(TAG) true
	# docker cp semantics changed between 1.7 and 1.8, so we cp the file to cwd and rename it.
	docker cp $(BUILD_IMAGE):/keepalived.tar.gz .
	docker rm -f $(BUILD_IMAGE)

push: container
	gcloud docker push $(PREFIX):$(TAG)

clean:
	rm -f kube-keepalived-vip
