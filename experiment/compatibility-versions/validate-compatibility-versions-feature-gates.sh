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
# against expected values defined in a versioned feature list.
#
# Usage: validate-compatibility-versions-feature-gates.sh <emulated_version> <current_version> <metrics_file> <feature_list> <prev_feature_list> <results_file>
set -o errexit -o nounset -o pipefail
# Check arg count
if [[ $# -ne 6 ]]; then
  echo "Usage: ${0} <emulated_version> <current_version> <metrics_file> <feature_list> <prev_feature_list> <results_file>"
  exit 1
fi
emulated_version="$1"   # e.g. "1.32"
current_version="$2"    # e.g. "1.33" - the actual version to check for DEPRECATED features
metrics_file="$3"       # path to /metrics
feature_list="$4"       # current versioned_feature_list.yaml
prev_feature_list="$5"       # previous versioned_feature_list.yaml
results_file="$6"
echo "Validating features for emulated_version=${emulated_version}, current_version=${current_version}..."
rm -f "${results_file}"
touch "${results_file}"

# Obtain the list of features that have been removed fully and are not in scope of emulation version(e.g kubelet)
REMOVED_FEATURE_LIST=${REMOVED_FEATURE_LIST:-''}

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

# Build the "expected" sets from previous versioned_feature_list.yaml
# => expected_stage[featureName], expected_lock[featureName], expected_value[featureName]
declare -A expected_stage
declare -A expected_lock
declare -A expected_value

prev_feature_stream="$(
  yq e -o=json '.' "${prev_feature_list}" \
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
done < <(echo "$prev_feature_stream")


# For each "expected" feature (versioned):
# - If missing from /metrics => fail unless stage==ALPHA or lock==true or is a deprecated feature that is listed as properly removed.
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

    # Deprecated feature that was removed, these can continue. These can only be features that are not in the 
    # scope of compatibility version(e.g kuebelet).
    if [[  "$REMOVED_FEATURE_LIST" == *"$feature_name"* ]]; then
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



declare -A current_stage
declare -A current_lock
declare -A current_value

current_feature_stream="$(
  yq e -o=json '.' "${feature_list}" \
    | jq -c '.[]'
)"

while IFS= read -r feature_entry; do
  feature_name=$(echo "${feature_entry}" | jq -r '.name')
  specs_json=$(echo "${feature_entry}"   | jq -c '.versionedSpecs')
  # We want the spec matching or below the emulated_version
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
  raw_stage=$(echo "$target_spec"       | jq -r '.preRelease')
  lockToDefault=$(echo "$target_spec"   | jq -r '.lockToDefault')
  defaultVal=$(echo "$target_spec"      | jq -r '.default')
  # Convert defaultVal (true/false) -> 1/0
  want="0"
  if [[ "$defaultVal" == "true" ]]; then
    want="1"
  fi
  current_stage["$feature_name"]="${raw_stage^^}"  # uppercase
  current_lock["$feature_name"]="$lockToDefault"
  current_value["$feature_name"]="$want"
done < <(echo "$current_feature_stream")

# Parse exact version matches to identify deprecated features
declare -A current_version_stage
# Parse features to check for version 1.0 GA features
declare -A version_1_0_stage

while IFS= read -r feature_entry; do
  feature_name=$(echo "${feature_entry}" | jq -r '.name')
  specs_json=$(echo "${feature_entry}"   | jq -c '.versionedSpecs')
  
  # Check for version 1.0 features
  version_1_0_spec="$(
    echo "${specs_json}" \
    | jq -r '
        [ .[]
          | select(
              (.version | sub("^v"; "") | tonumber) == 1.0
            )
        ]
        | if length > 0 then .[0] else null end
      '
  )"
  if [[ -n "$version_1_0_spec" && "$version_1_0_spec" != "null" ]]; then
    ga_stage=$(echo "$version_1_0_spec" | jq -r '.preRelease')
    version_1_0_stage["$feature_name"]="${ga_stage^^}"  # uppercase
  fi
  
  # We want the spec matching EXACTLY the current_version
  current_version_spec="$(
    echo "${specs_json}" \
    | jq -r --arg ver "${current_version}" '
        [ .[]
          | select(
              ( .version | sub("^v"; "") | tonumber )
              ==
              ($ver | sub("^v"; "") | tonumber)
            )
        ]
        | if length > 0 then .[0] else null end
      '
  )"
  # If no exact matching spec, skip
  if [[ -z "$current_version_spec" || "$current_version_spec" == "null" ]]; then
    continue
  fi
  # Get the stage for this exact version
  exact_stage=$(echo "$current_version_spec" | jq -r '.preRelease')
  current_version_stage["$feature_name"]="${exact_stage^^}"  # uppercase
done < <(echo "$current_feature_stream")

# For each actual feature in /metrics not in the "expected" map,
#  - if it's "1", we fail as "unexpected feature". because new gates not found in previous
#    expected gates can only be introduced if they are off by default (0) but not on by default (1)
  #    UNLESS:
  #      - new feature is a client-go feature then we do not fail but continue
  #      - new feature is a actually pre-existing code now being deprecated and as such called a "feature" retroactively for deprecation
for feature_name in "${!actual_features[@]}"; do
  if [[ -z "${expected_stage[$feature_name]:-}" ]]; then
    got="${actual_features[$feature_name]}"
    if [[ "$got" == "1" ]]; then
      # Check to see if gate is found in client-go and if so, continue
      if grep -q "$feature_name" staging/src/k8s.io/client-go/features/known_features.go; then
        continue
      fi
      # Check if gate is:
      # - 1. DEPRECATED feature gate at the same version as the binary version
      # - 2. GA feature with version 1.0 (used for code deprecation backfilling)
      # - 3. BETA feature with version 1.0 (used for bug fixes bound to binary version instead of emulation version)
      if [[ -n "${current_version_stage[$feature_name]:-}" == "DEPRECATED" ]] || \
         [[ -n "${version_1_0_stage[$feature_name]:-}" == "BETA" ]] || \
         [[ -n "${version_1_0_stage[$feature_name]:-}" == "GA" ]]; then
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