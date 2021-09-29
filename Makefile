# Copyright 2021 The Kubernetes Authors.
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

################################################################################
# ========================== Capture Environment ===============================
# get the repo root and output path
REPO_ROOT:=${CURDIR}
OUT_DIR=$(REPO_ROOT)/out
################################################################################
# ========================= Setup Go ===========================================
# go1.9+ can autodetect GOROOT, but if some other tool sets it ...
GOROOT:=
# enable modules
GO111MODULE=on
export GOROOT GO111MODULE
################################################################################
# ================================= Testing ====================================
# unit tests (hermetic)
unit:
	hack/make-rules/go-test/unit.sh
# integration tests
integration:
	hack/make-rules/go-test/integration.sh
# all tests
test:
	hack/make-rules/go-test/all.sh
################################################################################
# ================================= Cleanup ====================================
# standard cleanup target
clean:
	rm -rf "$(OUT_DIR)/"
################################################################################
# ============================== Auto-Update ===================================
# update generated code, gofmt, etc.
update:
	hack/make-rules/update/all.sh
# update generated code
generate:
	hack/make-rules/update/generated.sh
# gofmt
gofmt:
	hack/make-rules/update/gofmt.sh
################################################################################
# ================================== Linting ===================================
# run linters, ensure generated code, etc.
verify:
	hack/make-rules/verify/all.sh
# code linters
lint:
	hack/make-rules/verify/lint.sh
# shell linter
shellcheck:
	hack/make-rules/verify/shellcheck.sh
#################################################################################
.PHONY: unit test
