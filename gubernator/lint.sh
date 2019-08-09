#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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

set -o errexit -o pipefail -o nounset
export PYTHONPATH="third_party:${GAE_ROOT}:${GAE_ROOT}/lib/webapp2-2.5.2:${GAE_ROOT}/lib/jinja2-2.6"
cd "$(dirname "$0")"
pylint ../gubernator
shopt -s extglob
status=0
for f in templates/!(base).html; do
  if ! grep -q "% extends 'base.html'" "${f}"; then
    status=1
    echo "ERROR: ${f} should begin with '% extends 'base.html'"
  fi
done
exit ${status}
