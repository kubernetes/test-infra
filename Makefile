all: push

TAG = 0.6

mungegithub:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' ./mungegithub.go

container: mungegithub
	docker build -t gcr.io/google_containers/mungegithub:$(TAG) .

push: container
	gcloud docker push gcr.io/google_containers/mungegithub:$(TAG)

clean:
	rm -f mungegithub
