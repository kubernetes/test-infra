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

# This is a wrapper that converts the stdin/stdout part of the CNI
# calling convention into an input-file & output-file calling
# convention.  All the stuff written to stdout and stderr by the inner
# script is saved to a file in /tmp/.

INPUT=/tmp/c2d-$$-in
RESULT=/tmp/c2d-$$-out
LOG=/tmp/c2d-$$-log
cat > "${INPUT}"
"${0}-inner" "${INPUT}" "${RESULT}" &> "${LOG}"
RC=$?
if [ "${RC}" == "0" ]; then
    cat "${RESULT}"
else
    cat <<EOF
{
  "cniVersion": "0.1.0",
  "code": "${RC}",
  "msg": "${0}-inner returned ${RC}",
  "details": $(jq -R -s . < "${LOG}")
}
EOF
    exit "${RC}"
fi
