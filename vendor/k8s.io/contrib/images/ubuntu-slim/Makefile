all: push

TAG ?= 0.3
PREFIX ?= gcr.io/google_containers/ubuntu-slim
BUILD_IMAGE ?= ubuntu-build
TAR_FILE ?= rootfs.tar

container:
	rm -f $(TAR_FILE)
	docker build -t $(BUILD_IMAGE) -f Dockerfile.build .
	docker create --name $(BUILD_IMAGE) $(BUILD_IMAGE)
	docker export $(BUILD_IMAGE) > $(TAR_FILE)
	docker rm $(BUILD_IMAGE)
	docker build -t $(PREFIX):$(TAG) .

push: container
	docker push $(PREFIX):$(TAG)

clean:
	docker rmi -f $(PREFIX):$(TAG)
	docker rmi -f $(BUILD_IMAGE)
	rm -f $(TAR_FILE)
