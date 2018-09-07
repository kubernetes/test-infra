# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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