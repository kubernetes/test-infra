all: push

mungegithub:
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' ./mungegithub.go

container: mungegithub
	docker build -t gcr.io/google_containers/mungegithub:0.5 .

push: container
	gcloud docker push gcr.io/google_containers/mungegithub:0.5

clean:
	rm -f mungegithub
