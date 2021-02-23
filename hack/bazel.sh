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

# runs bazel and then coalesce.py (to convert results to junit)

set -o nounset
set -o errexit
set -o pipefail

# NEVER MERGE THIS, MOCKING FOR INVESTIGATING SPYGLASS PERFORMANCE ON LARGE NUBMER OF FILES
for i in $(seq 1 1000); do
    cat > "${ARTIFACTS}/junit_${i}.xml" << EOL
<testsuites>
    <testsuite name="pytest" errors="0" failures="0" skipped="0" tests="1" time="0.539" timestamp="2021-02-22T20:04:01.889404">
        <testcase classname="some_class" name="some_test_${i}" time="0.003"/>
    </testsuite>
</testsuites>
EOL
done

exit 1
