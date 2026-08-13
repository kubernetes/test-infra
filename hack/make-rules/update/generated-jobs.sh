#!/usr/bin/env bash
# Copyright 2024 The Kubernetes Authors.
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

echo "Installing requirements3.txt"
hack/run-in-python-container.sh \
    pip3 install -r requirements3.txt

echo "Generate jobs"
hack/run-in-python-container.sh \
    python3 hack/generate-jobs.py config/jobs/kubernetes/sig-node/*.conf

echo "Generate kOps jobs"
hack/run-in-python-container.sh \
    python3 config/jobs/kubernetes/kops/build_jobs.py
