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
OUT_DIR=$(REPO_ROOT)/_output
################################################################################
# ========================= Setup Go With Gimme ================================
# go version to use for build etc.
# go1.9+ can autodetect GOROOT, but if some other tool sets it ...
GOROOT:=
# enable modules
GO111MODULE=on
export PATH GOROOT GO111MODULE
# work around broken PATH export
SPACE:=$(subst ,, )
SHELL:=env PATH=$(subst $(SPACE),\$(SPACE),$(PATH)) $(SHELL)
################################################################################
# ================================= Testing ====================================
# unit tests (hermetic)
unit: go-unit py-unit
.PHONY: unit
go-unit:
	hack/make-rules/go-test/unit.sh
.PHONY: go-unit
py-unit:
	hack/make-rules/py-test/all.sh
.PHONY: py-unit
# integration tests
# integration:
#	hack/make-rules/go-test/integration.sh
# all tests
test: unit
.PHONY: test
################################################################################
# ================================= Cleanup ====================================
# standard cleanup target
clean:
	rm -rf "$(OUT_DIR)/"
################################################################################
# ============================== Auto-Update ===================================
# update generated code, gofmt, etc.
# update:
#	hack/make-rules/update/all.sh
# update generated code
#generate:
#	hack/make-rules/update/generated.sh
# gofmt
#gofmt:
#	hack/make-rules/update/gofmt.sh
################################################################################
# ================================== Linting ===================================
# run linters, ensure generated code, etc.
verify:
	hack/make-rules/verify/all.sh
# typescript linting
verify-tslint:
	hack/make-rules/verify/tslint.sh
.PHONY: verify-tslint
# go linters
go-lint:
	hack/make-rules/verify/golangci-lint.sh
update-gofmt:
	hack/make-rules/update/gofmt.sh
verify-gofmt:
	hack/make-rules/verify/gofmt.sh
update-file-perms:
	hack/make-rules/update/file-perms.sh
verify-file-perms:
	hack/make-rules/verify/file-perms.sh
update-spelling:
	hack/make-rules/update/misspell.sh
verify-spelling:
	hack/make-rules/verify/misspell.sh
update-labels:
	hack/make-rules/update/labels.sh
verify-labels:
	hack/make-rules/verify/labels.sh
.PHONY: update-codegen
update-codegen:
	hack/make-rules/update/codegen.sh
.PHONY: verify-codegen
verify-codegen:
	hack/make-rules/verify/codegen.sh
#################################################################################
.PHONY: verify go-lint update-gofmt verify-gofmt update-file-perms verify-file-perms update-spelling verify-spelling update-labels verify-labels
