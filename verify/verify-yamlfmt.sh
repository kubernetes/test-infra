#!/usr/bin/env bash

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

# Copies all YAML files into a temp directory, runs yamlfmt on these files,
# and then compares against the YAML files in the repo.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR=${ROOT_DIR:-$(dirname "${BASH_SOURCE}")/..}
# Changing into the root dir makes for nicer filenames than using ${ROOT_DIR}
# everywhere.
cd "${ROOT_DIR}"

export TMPDIR=$(mktemp -d "./_tmp-verify-yamlfmt.XXX")
trap "rm -r ${TMPDIR}" EXIT

yaml_files=$(find . -iname '*.yaml' -not -wholename '*/_tmp*')

echo "${yaml_files[@]}" | xargs cp --parents -t "${TMPDIR}"
ROOT_DIR=${TMPDIR} "./verify/update-yamlfmt.sh"

result=0
for f in ${yaml_files[@]}; do
  if ! diff -q "./$f" "${TMPDIR}/$f" >/dev/null; then
    result=1
    echo "$f is improperly formatted"
  fi
done

if [[ "${result}" -ne 0 ]]; then
  echo -e '\n!!! Please run verify/update-yamlfmt.sh'
fi
exit ${result}
