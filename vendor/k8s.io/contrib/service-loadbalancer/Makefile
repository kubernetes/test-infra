all: push

# 0.0 shouldn't clobber any released builds
TAG ?= 0.4
PREFIX ?= gcr.io/google_containers/servicelb
GCLOUD ?= gcloud
HAPROXY_IMAGE = contrib-haproxy
SRC = service_loadbalancer.go loadbalancer_log.go

service_loadbalancer: $(SRC)
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o $@ $(SRC)

container: service_loadbalancer
	docker build -t $(PREFIX):$(TAG) .

push: container
	$(GCLOUD) docker push $(PREFIX):$(TAG)

clean:
	# remove servicelb and contrib-haproxy images
	docker rmi -f $(HAPROXY_IMAGE):$(TAG) || true
	docker rmi -f $(PREFIX):$(TAG) || true
	rm -f service_loadbalancer

.PHONY: container push
