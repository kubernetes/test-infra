#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
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

# TODO(fejta): make this a good program, not bash

set -o errexit
set -o nounset
set -o pipefail

if [[ "$#" == 0 ]]; then
  echo "Usage: $(basename "$0") [json creds] <name>" >&2
  exit 1
fi

if [[ $# == 2 ]]; then
  creds="$1"
  shift
else
  creds=
fi
name="$1"

if [[ -n "$creds" ]]; then
  echo "Activating $creds..." >&2
  if [[ ! -f "$creds" ]]; then
    echo "Not found: $creds" >&2
    exit 1
  fi
  gcloud auth activate-service-account --key-file="$creds"
  gcloud auth list
  duration=20m
else
  duration=1m # Need shorter wait here as metadata server has its own caching
fi

create=yes

print-token() {
  gcloud config config-helper --force-auth-refresh --format='value(credential.access_token)'
}

# Format of the cookiefile is:
# * one line per cookie
# * tab separate the following fields:
#   - DOMAIN
#   - INITIAL_DOT
#   - PATH
#   - PATH_SPECIFIED
#   - expires
#   - name
#   - value

print-cookie() {
  if [[ "$#" != 4 ]]; then
    echo "Usage: print-cookie <HOST> <IS_DOT> <EXPIRES_EPOCH> <TOKEN>" >&2
    return 1
  fi
  host="$1"
  dot="$2"
  exp="$3"
  tok="$4"
  for part in "$host" "$dot" / TRUE "$exp" o; do
    echo -n ${part}$'\t' # apparently $'\t' is tab
  done
  echo "$tok"
}


while true; do
  token=$(print-token)
  # TODO(fejta): parse credential.token_expiry, wait until shortly before then.
  expire=$(expr 60 \* 60 + $(date +%s))
  echo -n "token expires at "
  date -d "@$expire"
  print-cookie .googlesource.com TRUE "$expire" "$token" > cookies
  print-cookie source.developers.google.com FALSE "$expire" "$token" >> cookies
  echo -n "cookies hash: "
  md5sum cookies

  kubectl create secret generic "$name" --from-file=cookies --dry-run -o yaml > secret.yaml
  if ! kubectl get -f secret.yaml; then
    verb=create
  else
    verb=replace
  fi
  kubectl "$verb" -f secret.yaml
  echo "successfully updated token, sleeping for $duration..."
  sleep "$duration"
done
