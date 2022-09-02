# Copyright 2020 The Kubernetes Authors.
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

.PHONY: build
build:
	go build .

.PHONY: test
test:
	go test ./...

push:
  ../../../hack/make-rules/go-run/arbitrary.sh run ./images/builder --project=k8s-staging-test-infra --scratch-bucket=gs://k8s-staging-test-infra-scratch triage

STATIC_FILES=index.html interactive.js model.js render.js style.css

# these need to be run by k8s-infra-prow or community members with access to the kubernetes.io GCP org
push-static:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp $(STATIC_FILES) gs://k8s-triage/

push-staging:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp $(STATIC_FILES) gs://k8s-triage/staging/

# these need to be run by k8s-prow or googlers with access to the google.com GCP org
redirect-static:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -a public-read redirect/index.html gs://k8s-gubernator/triage/

redirect-staging:
	gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -a public-read redirect/index.html gs://k8s-gubernator/triage/staging/

.PHONY: push push-static push-staging redirect-static redirect-staging

