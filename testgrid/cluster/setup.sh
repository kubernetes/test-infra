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

set -o errexit

# Ensure namespace exists
echo -n 'testgrid namespace: ' >&2
kubectl get namespaces/testgrid &>/dev/null || kubectl create namespace testgrid
echo 'PRESENT' >&2
# Ensure secrets exists
for i in updater configurator; do
  echo -n "testgrid-${i}-service-account secret: " >&2
  kubectl get secrets/testgrid-${i}-service-account --namespace=testgrid &>/dev/null \
    && echo 'PRESENT' >&2 \
    && continue
  echo 'MISSING' >&2
  echo 'Fix with the following:' >&2
  echo "  kubectl create secret generic testgrid-${i}-service-account \\" >&2
  echo '    --from-file=service-account.json=PATH/TO/SERVICE-ACCOUNT.json' >&2
  exit 1
done
echo 'READY to deploy with the following command:' >&2
echo '  bazel run //testgrid/cluster:dev.create'
