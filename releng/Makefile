# Copyright 2022 The Kubernetes Authors.
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

REPO_ROOT:=${CURDIR}/..

# ARGS can be overridden by calling make
ARGS ?=

prepare-release-branch:
	./run.sh

generate-tests:
	cd $(REPO_ROOT) && go run ./releng/generate-tests $(ARGS)

test-generate-tests:
	cd $(REPO_ROOT) && go test ./releng/generate-tests/...

test-prepare-release-branch:
	cd $(REPO_ROOT) && go test ./releng/prepare-release-branch/...

test: test-generate-tests test-prepare-release-branch

.PHONY: test generate-tests test-generate-tests test-prepare-release-branch prepare-release-branch
