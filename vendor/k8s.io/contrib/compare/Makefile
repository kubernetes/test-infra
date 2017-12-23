OUT_DIR = build
PACKAGE = k8s.io/contrib/compare

all: compare

deps:
	go get -u github.com/tools/godep

compare: src/compare-logs.go src/compare-metrics.go src/compare-resources.go src/scraper.go src/main/result-comparator.go deps 
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 godep go build -a -o build/compare src/main/result-comparator.go

test: src
	godep go test ${PACKAGE}/src -v