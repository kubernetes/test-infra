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
BIN_DIR:=$(REPO_ROOT)/_bin

all: prepare-release-branch

prepare-release-branch:
	cd $(REPO_ROOT) && source hack/build/setup-go.sh && \
	go build -o $(BIN_DIR)/prepare-release-branch $(REPO_ROOT)/releng/prepare-release-branch && \
	$(BIN_DIR)/prepare-release-branch

test:
	cd $(REPO_ROOT) && go test ./releng/config-forker/... ./releng/config-rotator/... ./releng/prepare-release-branch/...

.PHONY: all test prepare-release-branch
