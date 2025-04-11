#!/bin/bash

# Copyright 2025 The Kubernetes Authors.
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

FORK_TO_TEST="${FORK_TO_TEST:-kubernetes/perf-tests}"
BRANCH_TO_TEST="${BRANCH_TO_TEST:-master}"
IMAGE_TO_TEST="${IMAGE_TO_TEST:-ghcr.io/ritwikranjan/nptest:latest}"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo "Forking the repository..."
git clone "https://github.com/$FORK_TO_TEST.git"
cd $(basename $FORK_TO_TEST)
git checkout "$BRANCH_TO_TEST"

echo "Navigating to network/benchmarks/netperf directory..."
cd network/benchmarks/netperf
echo "Running netperf test..."
go run launch.go -image=$IMAGE_TO_TEST -kubeConfig=$KUBECONFIG -testFrom=0 -testTo=1 -json

echo "Cleaning up..."
cd ../../../..
rm -rf $(basename $FORK_TO_TEST)
echo "Script execution completed."
