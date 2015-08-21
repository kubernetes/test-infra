all: push

mungegithub: mungegithub.go
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w' ./mungegithub.go

container: mungegithub
	docker build -t gcr.io/google_containers/mungegithub:0.3 .

push: container
	gcloud docker push gcr.io/google_containers/mungegithub:0.3

clean:
	rm -f mungegithub
