# Copyright 2016 The Kubernetes Authors. All rights reserved#
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
# limitations under the License..

# Procedure for change:
# 1. Bump the tag number.
# 2. Create an image for the kubernetes user on Dockerhub and push there: make build push
# 3. Temporarily adjust any config files etc. that reference gcr.io/google_containers/kubelet-to-gcm:$(TAG)
#    to use kubernetes/addon-resizer:$(TAG) instead.
# 4. Test etc.
# 5. Issue PR.
# 6. When PR is approved make the gcr.io version of the image: make build push
# 7. Revert the references to kubernetes/kubelet-to-gcm:$(TAG) to gcr.io/google_containers/kubelet-to-gcm:$(TAG) 

OUT_DIR = build
PACKAGE = k8s.io/contrib/kubelet-to-gcm
#PREFIX = gcr.io/google_containers
PREFIX = gcr.io/gke-test-us-central1-b-0
TAG = 1.2

# Rules for building the real image for deployment to gcr.io

deps:
		go get -u github.com/tools/godep

compile: monitor/kubelet monitor/controller deps
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 godep go build -a -o $(OUT_DIR)/monitor monitor/main/daemon.go

test: monitor/kubelet monitor/controller
		godep go test ${PACKAGE}/monitor ${PACKAGE}/monitor/kubelet ${PACKAGE}/monitor/controller ${PACKAGE}/monitor/config -v

go: compile test

clean:
	rm -rf build

build: go
	docker build -t ${PREFIX}/kubelet-to-gcm:$(TAG) .

docker: build

push: docker
	gcloud docker push ${PREFIX}/kubelet-to-gcm:$(TAG)
