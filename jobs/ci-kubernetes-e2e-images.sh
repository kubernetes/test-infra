#!/bin/bash
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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# compute current md5 for all e2e-image dependent files
echo "START"
touch e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/kubernetes/master/cluster/get-kube.sh | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/kubernetes/master/hack/e2e.go | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/kubernetes/master/hack/jenkins/upload-to-gcs.sh | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/kubernetes/master/third_party/forked/shell2junit/sh2ju.sh | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/test-infra/master/jenkins/e2e-image/e2e-runner.sh | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/test-infra/master/jenkins/e2e-image/Dockerfile | md5sum | awk '{print $1}' >> e2e-hash-local.txt
curl -s https://raw.githubusercontent.com/kubernetes/test-infra/master/jenkins/e2e-image/Makefile | md5sum | awk '{print $1}' >> e2e-hash-local.txt
md5sum e2e-hash-local.txt | awk '{print $1}'

# Get md5 from gcp
gsutil cp gs://k8s-testimages-misc/e2e-hash.txt e2e-hash.txt
md5sum e2e-hash.txt | awk '{print $1}'

DIFF=$(diff e2e-hash.txt e2e-hash-local.txt || true)

# clean up
rm e2e-hash.txt
rm e2e-hash-local.txt

# if changed, update new md5, make new images
if [ "$DIFF" ] 
then
  # Upload new hash to gcs
  echo "NEED TRIGGER NEW BUILD!"
  gsutil cp e2e-hash-local.txt gs://k8s-testimages-misc/e2e-hash.txt
  
  # cd jenkins/e2e-images
  # make build

  # test images
  # <test code here>

  # make push
fi
echo "FINISH"

