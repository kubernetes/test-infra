#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

# shellcheck disable=SC2034
readonly DEFAULT_CLUSTER_NAME="kind-prow-integration"
readonly DEFAULT_CONTEXT="kind-${DEFAULT_CLUSTER_NAME}"
readonly DEFAULT_REGISTRY_NAME="kind-registry"
readonly DEFAULT_REGISTRY_PORT="5001"
readonly PROW_COMPONENTS=(
  crier
  deck
  deck-tenanted
  fakegerritserver
  fakeghserver
  hook
  horologium
  prow-controller-manager
  sinker
  tide
)
