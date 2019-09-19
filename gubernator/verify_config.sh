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

# This script verifies the Gubernator configuration
# file is in sync with Prow.

cd "$( dirname "${BASH_SOURCE[0]}" )"
config="$( mktemp )"
trap "rm ${config}" EXIT

cp ./config.yaml "${config}"
./update_config.py ./../config/prow/config.yaml ./../config/jobs "${config}"

if ! output="$( diff ./config.yaml "${config}" )"; then
    echo "Gubernator configuration file is out of sync!"
    echo "${output}"
    echo "Run \`gubernator/update_config.sh\`"
    exit 1
fi