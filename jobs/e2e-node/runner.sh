#!/bin/bash

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

# Script executed by jenkins to run node e2e tests against gce
# Usage: jobs/e2e-node/runner.sh <path to properties>
# Properties files:
# - jobs/e2e-node/ci-ubuntu-ci.properties: for running Ubuntu node e2e tests

set -e
set -x

: "${1:?Usage jobs/e2e-node/runner <path to properties>}"

. $1

# indirectly generates test/e2e/generated/bindata.go too
#make generated_files

# TODO converge build steps with hack/build-go some day if possible.
go build test/e2e_node/environment/conformance.go

PARALLELISM=${PARALLELISM:-8}
WORKSPACE=${WORKSPACE:-"/tmp/"}
ARTIFACTS=${WORKSPACE}/_artifacts
TIMEOUT=${TIMEOUT:-"45m"}

mkdir -p ${ARTIFACTS}

go run test/e2e_node/runner/remote/run_remote.go  --logtostderr --vmodule=*=4 \
  --ssh-env="gce" --ssh-user="$GCE_USER" --zone="$GCE_ZONE" --project="$GCE_PROJECT" \
  --hosts="$GCE_HOSTS" --images="$GCE_IMAGES" --image-project="$GCE_IMAGE_PROJECT" \
  --image-config-file="$GCE_IMAGE_CONFIG_PATH" --cleanup="$CLEANUP" \
  --results-dir="$ARTIFACTS" --ginkgo-flags="--nodes=$PARALLELISM $GINKGO_FLAGS" \
  --test-timeout="$TIMEOUT" --test_args="$TEST_ARGS --kubelet-flags=\"$KUBELET_ARGS\"" \
  --instance-metadata="$GCE_INSTANCE_METADATA" --system-spec-name="$SYSTEM_SPEC_NAME"
