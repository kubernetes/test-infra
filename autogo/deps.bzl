# Copyright 2018 The Kubernetes Authors.
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

def autogo_dependencies():
  """Ensure we load all the rules autogo depends on (mainly go/gazelle)."""
  if "io_bazel_rules_go" not in native.existing_rules():
    print("Creating @io_bazel_rules_go...")
    native.git_repository(
        name = "io_bazel_rules_go",
        tag = "0.11.0",
        remote = "https://github.com/bazelbuild/rules_go.git")
