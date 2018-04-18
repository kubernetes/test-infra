# Copyright 2016 The Kubernetes Authors.
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

IMAGE ?= gcr.io/k8s-test-infra-aws/aws-janitor
VERSION ?= $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

image:
	CGO_ENABLED=0 go build k8s.io/test-infra/maintenance/aws-janitor
	docker build -t "$(IMAGE):$(VERSION)" .

push: image
	docker tag "$(IMAGE):$(VERSION)" "$(IMAGE):latest"
	docker push "$(IMAGE):$(VERSION)"
	docker push "$(IMAGE):latest"

.PHONY: image push
