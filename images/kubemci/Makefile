# Copyright 2018 The Kubernetes Authors.
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

VERSION := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

IMG = "gcr.io/k8s-testimages/e2e-kubemci"

# Build a versioned image.
image:
	docker build --no-cache -t "$(IMG):$(VERSION)" --build-arg "IMAGE_ARG=$(IMG):$(VERSION)" .

# Build both the versioned and latest images.
image-latest: image
	docker build --no-cache -t "$(IMG):latest" --build-arg "IMAGE_ARG=$(IMG):latest" .

# Build and push the versioned image.
push: image
	# TODO(krzyzacy): https://github.com/GoogleCloudPlatform/docker-credential-gcr/issues/21#issuecomment-365803776
	# gcloud auth fails in CI, this image build file runs in CI only
	# so use gcloud docker -- push for now until the above issue resolves.

	# gcloud beta auth configure-docker
	# docker push "$(IMG):$(VERSION)"
	gcloud docker -- push "$(IMG):$(VERSION)"

# Build and push both the versioned and latest images.
push-latest: push image-latest
	# gcloud beta auth configure-docker
	# docker push "$(IMG):latest"
	gcloud docker -- push "$(IMG):latest"

.PHONY: image image-latest push push-latest
