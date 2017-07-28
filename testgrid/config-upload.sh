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

# Create and duplicate build under the go path

# GOPATH should point to /go here
mkdir -p $GOPATH/src/k8s.io/
cp -r $WORKSPACE $GOPATH/src/k8s.io/
export TEST_INFRA=$GOPATH/src/k8s.io/test-infra

# Export config
export CONFIGDIR=$TEST_INFRA/testgrid/config
go run $CONFIGDIR/main.go --yaml=$CONFIGDIR/config.yaml --output=$CONFIGDIR/config
gsutil cp $CONFIGDIR/config gs://k8s-testgrid/config
rm $CONFIGDIR/config

# Remove duplicated build
rm -rf $TEST_INFRA
