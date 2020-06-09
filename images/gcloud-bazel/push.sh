#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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


OLD=3.0.0
NEW=3.1.0

cd "$(dirname "${BASH_SOURCE[0]}")" || exit
rm -rf rules_k8s
git clone https://github.com/bazelbuild/rules_k8s.git
make -C rules_k8s/images/gcloud-bazel push PROJECT=k8s-testimages "OLD=$OLD" "NEW=$NEW"
rm -rf rules_k8s
