#!/usr/bin/env bash
# Copyright The Kubernetes Authors.
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

# Mirrors single-select field values from a source GitHub Projects v2 board
# (e.g. the SIG Release Enhancement Tracking board, project 264) onto a target
# board (e.g. the SIG Node KEP tracking board, project 265), translating field
# and option names per a JSON mapping file.
#
# Semantics:
#   * Read-only on the source project.
#   * Updates ONLY items already present on the target project.
#   * Never adds items to or removes items from the target.
#   * Skips items not found on the source (logged as a warning).
#   * Skips fields whose translated option has no equivalent on the target.
#
# Required environment variables:
#   GH_TOKEN                   classic PAT with scopes: read:org, project
#                              (must be SSO-authorized for the org)
#   ORGANIZATION               e.g. "kubernetes"
#   SOURCE_PROJECT_NUMBER      e.g. "264"
#   TARGET_PROJECT_NUMBER      e.g. "265"
#
# Optional environment variables:
#   MAPPING_FILE   path to a JSON mapping (default: ./mapping.json next to this script)
#   DRY_RUN        if "1", log intended mutations but do not write
#   ONLY_ISSUE     restrict to one issue, e.g. "kubernetes/enhancements#1234"

set -eu -o pipefail

: "${GH_TOKEN:?GH_TOKEN is required}"
: "${ORGANIZATION:?ORGANIZATION is required}"
: "${SOURCE_PROJECT_NUMBER:?SOURCE_PROJECT_NUMBER is required}"
: "${TARGET_PROJECT_NUMBER:?TARGET_PROJECT_NUMBER is required}"

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
MAPPING_FILE="${MAPPING_FILE:-$SCRIPT_DIR/mapping.json}"
DRY_RUN="${DRY_RUN:-0}"
ONLY_ISSUE="${ONLY_ISSUE:-}"

[ -r "$MAPPING_FILE" ] || { echo "[Error] MAPPING_FILE not readable: $MAPPING_FILE" >&2; exit 1; }

log()  { printf '[%s] %s\n' "$(date -u +%FT%TZ)" "$*"; }
warn() { log "WARN: $*" >&2; }

# Strip the human-readable "_comment" key and normalize defaults.
MAPPING="$(jq 'with_entries(select(.key | startswith("_") | not))
               | with_entries(.value.target_field //= .key)
               | with_entries(.value.options     //= {})' "$MAPPING_FILE")"

project_meta() {
  local number="$1"
  gh api graphql --paginate -F org="$ORGANIZATION" -F number="$number" -f query='
    query($org: String!, $number: Int!) {
      organization(login: $org) {
        projectV2(number: $number) {
          id
          fields(first: 50) {
            nodes {
              ... on ProjectV2SingleSelectField {
                id name
                options { id name }
              }
            }
          }
        }
      }
    }'
}

project_items() {
  local number="$1"
  gh api graphql --paginate -F org="$ORGANIZATION" -F number="$number" -f query='
    query($org: String!, $number: Int!, $endCursor: String) {
      organization(login: $org) {
        projectV2(number: $number) {
          items(first: 100, after: $endCursor) {
            pageInfo { hasNextPage endCursor }
            nodes {
              id
              content {
                ... on Issue {
                  id number
                  repository { nameWithOwner }
                }
              }
              fieldValues(first: 20) {
                nodes {
                  ... on ProjectV2ItemFieldSingleSelectValue {
                    name
                    field { ... on ProjectV2SingleSelectField { name } }
                  }
                }
              }
            }
          }
        }
      }
    }'
}

log "Fetching source project metadata (#$SOURCE_PROJECT_NUMBER)..."
SRC_META="$(project_meta "$SOURCE_PROJECT_NUMBER")"
log "Fetching target project metadata (#$TARGET_PROJECT_NUMBER)..."
DST_META="$(project_meta "$TARGET_PROJECT_NUMBER")"
DST_PROJECT_ID="$(jq -r '.data.organization.projectV2.id' <<<"$DST_META")"

log "Fetching source project items..."
SRC_ITEMS="$(project_items "$SOURCE_PROJECT_NUMBER")"
log "Fetching target project items..."
DST_ITEMS="$(project_items "$TARGET_PROJECT_NUMBER")"

# Index source items by underlying issue node id -> {source field name -> option name}.
SRC_INDEX="$(jq '
  [ .data.organization.projectV2.items.nodes[]
    | select(.content.id != null)
    | { key: .content.id,
        value: ( [ .fieldValues.nodes[]
                   | select(.field.name != null)
                   | { (.field.name): .name } ] | add ) } ]
  | from_entries' <<<"$SRC_ITEMS")"

mapfile -t SRC_FIELDS < <(jq -r 'keys[]' <<<"$MAPPING")

updates=0
skips_missing=0
skips_no_option=0
noops=0

# Process substitution keeps the loop in the parent shell so counters persist.
while IFS= read -r row; do
  item_id="$(jq -r '.itemId'  <<<"$row")"
  issue_id="$(jq -r '.issueId' <<<"$row")"
  issue_ref="$(jq -r '"\(.repo)#\(.issueNo)"' <<<"$row")"

  if [ -n "$ONLY_ISSUE" ] && [ "$issue_ref" != "$ONLY_ISSUE" ]; then
    continue
  fi

  src_fields="$(jq --arg id "$issue_id" '.[$id] // empty' <<<"$SRC_INDEX")"
  if [ -z "$src_fields" ] || [ "$src_fields" = "null" ]; then
    warn "skip $issue_ref: not present on source project #$SOURCE_PROJECT_NUMBER"
    skips_missing=$((skips_missing + 1))
    continue
  fi

  for src_field in "${SRC_FIELDS[@]}"; do
    dst_field="$(jq -r --arg f "$src_field" '.[$f].target_field' <<<"$MAPPING")"

    src_val="$(jq -r --arg f "$src_field" '.[$f] // empty' <<<"$src_fields")"
    [ -z "$src_val" ] && continue

    translated_val="$(jq -r --arg f "$src_field" --arg v "$src_val" \
      '.[$f].options[$v] // $v' <<<"$MAPPING")"

    dst_val="$(jq -r --arg f "$dst_field" '.current[$f] // empty' <<<"$row")"
    if [ "$translated_val" = "$dst_val" ]; then
      noops=$((noops + 1))
      continue
    fi

    dst_field_id="$(jq -r --arg f "$dst_field" '
      .data.organization.projectV2.fields.nodes[]
      | select(.name == $f) | .id' <<<"$DST_META")"
    dst_option_id="$(jq -r --arg f "$dst_field" --arg o "$translated_val" '
      .data.organization.projectV2.fields.nodes[]
      | select(.name == $f)
      | .options[]? | select(.name == $o) | .id' <<<"$DST_META")"

    if [ -z "$dst_field_id" ] || [ -z "$dst_option_id" ]; then
      warn "skip $issue_ref field=\"$dst_field\": no \"$translated_val\" option on target (source \"$src_field\"=\"$src_val\")"
      skips_no_option=$((skips_no_option + 1))
      continue
    fi

    log "update $issue_ref \"$dst_field\": \"${dst_val:-<unset>}\" -> \"$translated_val\" (source \"$src_field\"=\"$src_val\")"
    updates=$((updates + 1))
    if [ "$DRY_RUN" = "1" ]; then
      continue
    fi
    gh api graphql --silent \
      -F project="$DST_PROJECT_ID" \
      -F item="$item_id" \
      -F field="$dst_field_id" \
      -F option="$dst_option_id" \
      -f query='
        mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
          updateProjectV2ItemFieldValue(input: {
            projectId: $project, itemId: $item, fieldId: $field,
            value: { singleSelectOptionId: $option }
          }) { projectV2Item { id } }
        }'
  done
done < <(jq -c '
  .data.organization.projectV2.items.nodes[]
  | select(.content.id != null)
  | { itemId: .id,
      issueId: .content.id,
      issueNo: .content.number,
      repo: .content.repository.nameWithOwner,
      current: ( [ .fieldValues.nodes[]
                   | select(.field.name != null)
                   | { (.field.name): .name } ] | add ) }' <<<"$DST_ITEMS")

log "Sync finished. updates=$updates noops=$noops skips_missing=$skips_missing skips_no_option=$skips_no_option dry_run=$DRY_RUN"
