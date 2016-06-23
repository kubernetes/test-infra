#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

STOP=${1:-"stop"}
PORT=${PORT:-9999}

SELF_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${SELF_DIR}/util.sh

port-forward &>/dev/null &
PID=$!
trap "kill $PID" EXIT

if ! wait-for-port; then
  exit 1
fi

echo "About to ${STOP} merges. (Options: 'stop' or 'resume')"
curl "localhost:${PORT}/api/emergency/${STOP}"
echo "" # output doesn't include a trailing newline
