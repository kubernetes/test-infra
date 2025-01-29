#!/usr/bin/env bash
# Copyright 2025 The Kubernetes Authors.
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

# This script validates feature gates from a Kubernetes cluster's /metrics endpoint
# against expected values defined in versioned and unversioned feature lists.
#
# Usage: validate-compatibility-versions-feature-gates.sh <emulated_version> <metrics_file> <versioned_feature_list> <unversioned_feature_list> <results_file>

set -o errexit -o nounset -o pipefail

# Check arg count
if [[ $# -ne 5 ]]; then
  echo "Usage: ${0} <emulated_version> <metrics_file> <versioned_feature_list> <unversioned_feature_list> <results_file>"
  exit 1
fi

emulated_version="$1"   # e.g. "1.32"
metrics_file="$2"       # path to /metrics
feature_list="$3"       # versioned_feature_list.yaml
unversioned_feature_list="$4" # unversioned_feature_list.yaml
results_file="$5"

echo "Validating features for ${emulated_version}..."
rm -f "${results_file}"
touch "${results_file}"

# Parse /metrics -> actual_features[featureName] = 0 or 1
declare -A actual_features
declare -A actual_stages

while IFS= read -r line; do
  # Example line:
  #   kubernetes_feature_enabled{name="DisableKubeletCloudCredentialProviders",stage="Alpha"} 1

  # Capture name in group [1], stage in [2], and numeric value (0 or 1) in [3].
  # NOTE: The capture group for stage="([^"]*)" matches any stage text (including empty).
  if [[ "$line" =~ ^kubernetes_feature_enabled\{name=\"([^\"]+)\",stage=\"([^\"]*)\"}.*\ ([0-9]+)$ ]]; then
    feature_name="${BASH_REMATCH[1]}"
    feature_stage="${BASH_REMATCH[2]}"
    feature_value="${BASH_REMATCH[3]}"

    # Store these in two separate maps
    actual_features["$feature_name"]="$feature_value"
    actual_stages["$feature_name"]="$feature_stage"
  fi
done < <(grep '^kubernetes_feature_enabled' "${metrics_file}")

# Build the "expected" sets from versioned_feature_list.yaml
# => expected_stage[featureName], expected_lock[featureName], expected_value[featureName]
declare -A expected_stage
declare -A expected_lock
declare -A expected_value

feature_stream="$(
  yq e -o=json '.' "${feature_list}" \
    | jq -c '.[]'
)"

while IFS= read -r feature_entry; do
  feature_name=$(echo "${feature_entry}" | jq -r '.name')
  specs_json=$(echo "${feature_entry}"   | jq -c '.versionedSpecs')

  # Numeric parse for .version vs emulated_version
  target_spec="$(
    echo "${specs_json}" \
    | jq -r --arg ver "${emulated_version}" '
        [ .[]
          | select(
              ( .version | sub("^v"; "") | tonumber )
              <=
              ($ver | sub("^v"; "") | tonumber)
            )
        ]
        | last
      '
  )"

  # If no matching spec, skip
  if [[ -z "$target_spec" || "$target_spec" == "null" ]]; then
    continue
  fi

  # Read fields
  raw_stage=$(echo "$target_spec"       | jq -r '.preRelease')
  lockToDefault=$(echo "$target_spec"   | jq -r '.lockToDefault')
  defaultVal=$(echo "$target_spec"      | jq -r '.default')

  # Convert defaultVal (true/false) -> 1/0
  want="0"
  if [[ "$defaultVal" == "true" ]]; then
    want="1"
  fi

  expected_stage["$feature_name"]="${raw_stage^^}"
  expected_lock["$feature_name"]="$lockToDefault"
  expected_value["$feature_name"]="$want"
done < <(echo "$feature_stream")

# Build the "expected_unversioned" sets from unversioned_feature_list.yaml
# => expected_unversioned_stage[featureName], expected_unversioned_lock[featureName], expected_unversioned_value[featureName]
declare -A expected_unversioned_stage
declare -A expected_unversioned_lock
declare -A expected_unversioned_value

unversioned_feature_stream="$(
  yq e -o=json '.' "${unversioned_feature_list}" \
    | jq -c '.[]'
)"

while IFS= read -r unversioned_feature_entry; do
  unversioned_feature_name=$(echo "${unversioned_feature_entry}" | jq -r '.name')
  unversioned_specs_json=$(echo "${unversioned_feature_entry}"   | jq -c '.versionedSpecs') # Although named versionedSpecs in YAML, it's unversioned list.

  # Unversioned should always use the first spec (assuming only one exists or first one is the default)
  target_unversioned_spec="$(
    echo "${unversioned_specs_json}" \
    | jq -r '.[0]' # Get the first spec
  )"

  # If no spec, skip (should not happen in valid file)
  if [[ -z "$target_unversioned_spec" || "$target_unversioned_spec" == "null" ]]; then
    continue
  fi

  # Read fields - these are default values for unversioned features
  raw_unversioned_stage=$(echo "$target_unversioned_spec"       | jq -r '.preRelease') # Can use this or default to GA
  unversioned_lockToDefault=$(echo "$target_unversioned_spec"   | jq -r '.lockToDefault')
  unversioned_defaultVal=$(echo "$target_unversioned_spec"      | jq -r '.default')

  # Convert defaultVal (true/false) -> 1/0
  unversioned_want="0"
  if [[ "$unversioned_defaultVal" == "true" ]]; then
    unversioned_want="1"
  fi

  expected_unversioned_stage["$unversioned_feature_name"]="$raw_unversioned_stage"
  expected_unversioned_lock["$unversioned_feature_name"]="$unversioned_lockToDefault"
  expected_unversioned_value["$unversioned_feature_name"]="$unversioned_want"
done < <(echo "$unversioned_feature_stream")


# For each "expected" feature (versioned):
# - If missing from /metrics => fail unless stage==ALPHA or lock==true
# - If present & stage!=ALPHA => compare numeric value
for feature_name in "${!expected_stage[@]}"; do
  stage="${expected_stage[$feature_name]}"
  locked="${expected_lock[$feature_name]}"
  want="${expected_value[$feature_name]}"

  got="${actual_features[$feature_name]:-}"  # empty if missing

  # If present, but stage==ALPHA => no checks are done
  if [[ "$stage" == "ALPHA" ]]; then
    continue
  fi

  if [[ -z "$got" ]]; then
    # Missing from metrics
    if [[ "$locked" == "true" ]]; then
      continue
    fi
    echo "FAIL: expected feature gate '$feature_name' not found in metrics (stage=${stage}, lockToDefault=${locked})" \
    >> "${results_file}"
    continue
  fi

  # If present, stage!=ALPHA => compare true/false enabled value
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: feature '$feature_name' expected value $want, got $got" \
      >> "${results_file}"
  fi
done

# For each "expected_unversioned" feature:
# - If missing from /metrics => fail unless stage==ALPHA or lock==true
# - If present => compare numeric value
for unversioned_feature_name in "${!expected_unversioned_stage[@]}"; do
  unversioned_stage="${expected_unversioned_stage[$unversioned_feature_name]}"
  unversioned_locked="${expected_unversioned_lock[$unversioned_feature_name]}"
  unversioned_want="${expected_unversioned_value[$unversioned_feature_name]}"

  got="${actual_features[$unversioned_feature_name]:-}"  # empty if missing

  # If present, but stage==ALPHA => no checks are done
  if [[ "$unversioned_stage" == "ALPHA" ]]; then
    continue
  fi

  if [[ -z "$got" ]]; then
    # Missing from metrics
    if [[ "$unversioned_locked" == "true" ]]; then
      continue
    fi
    echo "FAIL: expected unversioned feature gate '$unversioned_feature_name' not found in metrics (lockToDefault=${unversioned_locked})" \
    >> "${results_file}"
    continue
  fi

  # If present, compare true/false enabled value
  if [[ "$got" != "$unversioned_want" ]]; then
    echo "FAIL: unversioned feature '$unversioned_feature_name' expected value $unversioned_want, got $got" \
      >> "${results_file}"
  fi
done

# For each actual feature in /metrics not in the "expected" maps (versioned OR unversioned),
#  - if it's "1", we fail as "unexpected feature". because new gates not found in previous
#    expected gates can only be introduced if they are off by default (0) but not on by default (1)
  #    NOTE: if the new feature is a client-go feature then we do not fail but continue
for feature_name in "${!actual_features[@]}"; do
  if [[ -z "${expected_stage[$feature_name]:-}" ]] && [[ -z "${expected_unversioned_stage[$feature_name]:-}" ]]; then
    got="${actual_features[$feature_name]}"
    if [[ "$got" == "1" ]]; then
      # Check to see if gate is found in client-go and if so, continue
      if grep -q "$feature_name" staging/src/k8s.io/client-go/features/known_features.go; then
        continue
      fi
      echo "FAIL: unexpected feature '$feature_name' found in /metrics, got=1" \
        >> "${results_file}"
    fi
  fi
done


if grep -q "FAIL" "$results_file"; then
  echo "Validation failures detected"
  exit 1
fi