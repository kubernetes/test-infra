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

set -o errexit
set -o nounset
set -o pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $(basename "$0") <gcp-project-id>" >&2
    exit 1
fi
proj=$1

# TODO(fejta): always enable
goal="build:remote-$proj --config=remote --remote_instance_name=projects/$proj/instances/default_instance"

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
    echo "Application default: $GOOGLE_APPLICATION_CREDENTIALS"
elif [[ ! -f ~/.config/gcloud/application_default_credentials.json ]]; then
    echo "Remote execution requires application-default credentials..."
    (
      set -o xtrace
      gcloud auth application-default login
    )
fi

echo "Add the following line to ~/.bazelrc:"
echo "  $goal"
if ! grep "$goal" ~/.bazelrc &>/dev/null; then
    read -p "Update ~/.bazelrc file [y/N]: " conf
    case "$conf" in
        y*|Y*)
            ;;
        *)
            exit 1
            ;;
    esac

    touch ~/.bazelrc
    echo "$goal" >> ~/.bazelrc
fi

echo "Use by adding --config=remote-$proj to your bazel commands:"
echo "  bazel test --config=remote-$proj //... # etc"
