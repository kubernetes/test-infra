TAG = $(shell date -u +v%Y%m%d)-$(shell git describe --tags --always --dirty)

# Build and push specific variables.
REGISTRY ?= gcr.io
PROJECT  ?= knative-tests
PUSH     ?= docker push

coverage-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o coverage
	chmod +x coverage
	docker build -t "$(REGISTRY)/$(PROJECT)/test-infra/coverage-dev:$(TAG)" -t "$(REGISTRY)/$(PROJECT)/test-infra/coverage-dev:latest-dev" .
	$(PUSH) "$(REGISTRY)/$(PROJECT)/test-infra/coverage-dev:$(TAG)"
	$(PUSH) "$(REGISTRY)/$(PROJECT)/test-infra/coverage-dev:latest-dev"