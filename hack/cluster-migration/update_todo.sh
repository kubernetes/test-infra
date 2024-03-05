#!/usr/bin/env sh
# Copyright 2024 The Kubernetes Authors.
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

set -x -o pipefail

# Cd to the root of the repository
cd "$(git rev-parse --show-toplevel)"

# Run the cluster-migration script to generate the todo report
go run hack/cluster-migration/main.go --config config/prow/config.yaml --job-config config/jobs --todo-report

# Move the report to the docs folder
mv cluster-migration-todo.md docs/cluster-migration-todo.md

# Commit the changes and open a PR
git config user.name "k8s-infra-ci-robot"
git config user.email "k8s-infra-ci-robot@email.com"

branch=migration-report-$(date +'%m-%d-%Y')
git checkout -b $branch

git add docs/cluster-migration-todo.md
git commit -m "Update cluster migration todo report $(date +'%m-%d-%Y')"
git remote add k8s-infra-ci-robot git@github.com:k8s-infra-ci-robot/test-infra.git
git push -f k8s-infra-ci-robot ${branch}

gh pr create --fill --base master --head k8s-infra-ci-robot:${branch}