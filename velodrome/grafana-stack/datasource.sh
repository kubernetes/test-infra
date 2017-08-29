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

set -o nounset
set -o pipefail
set -o errexit

if [[ $# -ne 2 ]]; then
  echo "usage: $0 server_hostname grafana_admin_password" >&2
  exit 64
fi

server_hostname=$1
grafana_admin_password=$2

curl -s --fail "http://${server_hostname}/api/datasources/name/github" -u "admin:${grafana_admin_password}" ||
env - server_hostname="${server_hostname}" envsubst <<EOF |
{
  "name": "github",
  "type": "influxdb",
  "access": "proxy",
  "url": "http://${server_hostname}:8181/",
  "user": "grafana",
  "password": "password",
  "database": "github",
  "isDefault": true
}
EOF
curl \
  -X POST --fail \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  "http://${server_hostname}/api/datasources" \
  -u "admin:${grafana_admin_password}" \
  --data-binary @-
