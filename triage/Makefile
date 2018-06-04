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

IMG = gcr.io/k8s-testimages/triage
TAG := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)

build:
	docker build -t $(IMG):$(TAG) .
	docker tag $(IMG):$(TAG) $(IMG):latest
	echo Built $(IMG):$(TAG) and tagged with $(IMG):latest ;

push:
	docker push $(IMG):$(TAG)
	docker push $(IMG):latest
	echo Pushed $(IMG):latest and $(IMG):$(TAG)

push_static:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -Z -a public-read index.html interactive.js model.js render.js style.css gs://k8s-gubernator/triage/

push_staging:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -Z -a public-read index.html interactive.js model.js render.js style.css gs://k8s-gubernator/triage/staging/

.PHONY: all build push push_static

all: build
