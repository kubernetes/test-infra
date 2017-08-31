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

if [ "$#" -ne 0 ]; then
  echo "usage: $0"
  exit 1
fi

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! ($SED --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'."
  exit 1
fi

cd $(dirname $0)

function bump_component() {
	local component="$1"

	makefile_version_re="^\(${1}_VERSION.*=\s*\)"
	version=$($SED -n "s/$makefile_version_re//Ip" Makefile)
	new_version=$(awk -F. '{print $1 "." $2+1}' <<< $version)

	echo "program: $1"
	echo "old version: $version"
	echo "new version: $new_version"

	$SED -i "s/$makefile_version_re.*/\1$new_version/I" Makefile
	$SED -i "s/\(${1}:\)[0-9.]*/\1$new_version/I" cluster/*.yaml
}

commit_prefix="Bumped deployment versions for:"
last_bump="$( git log --grep="${commit_prefix}" --pretty=%H | head -n 1 )"
if [[ -z "${last_bump}" ]]; then
	# we haven't used the script yet, so we need to find the last
	# commit that changed a VERSION field in the Makefile instead
	for commit in $( git log --pretty=%H -- ./Makefile ); do
		if git diff "${commit}~1..${commit}" -- ./Makefile | grep -Eq "[+-].*VERSION"; then
			last_bump="${commit}"
			break
		fi
	done
fi

components=( $( find ./cmd/ -mindepth 1 -maxdepth 1 -type d -print0 | xargs --null -L 1 basename ) )
bumped_components=()
for component in "${components[@]}"; do
	if [[ -n "$( git diff "${last_bump}~1..HEAD" -- "./cmd/${component}/" "./${component}/" )" ]]; then
		bump_component "${component}"
		bumped_components+=( "${component}" )
	fi
done

# if we haven't already bumped hook, check for plugin updates
hook_bumped=0
for component in "${bumped_components[@]}"; do
	if [[ "${component}" == "hook" ]]; then
		hook_bumped=1
	fi
done

if [[ "${hook_bumped}" == 0 ]]; then
	if [[ -n "$( git diff "${last_bump}~1..HEAD" -- "./plugins/" )" ]]; then
		bump_component "hook"
		bumped_components+=( "hook" )
	fi
fi

if [[ "${#bumped_components[@]}" -gt 0 ]]; then
	git add Makefile cluster/*.yaml
	git commit -m "${commit_prefix} ${bumped_components[*]}"
fi
