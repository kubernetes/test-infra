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
# ================================ Building ===================================
DOCKER_IMAGE_NAME=pytest-infra

docker-build:
	docker build -t $(DOCKER_IMAGE_NAME) .

docker-run:
	docker run -v $(shell pwd):/app $(DOCKER_IMAGE_NAME)

pybuild:
	mkdir -p _bin-make
	make -C experiment/ build
	make -C metrics/ build
	make -C hack/ build
	make -C releng/ build

# all build
build: pybuild
################################################################################
# ================================= Testing ====================================
# unit tests (hermetic)
unit:
	hack/make-rules/go-test/unit.sh

pytests:
	mkdir -p _bin-make
	make -C kettle/ test
	make -C metrics/ test
	make -C hack/ test
	make -C releng/ test

# integration tests
# integration:
#	hack/make-rules/go-test/integration.sh
# all tests
test: unit pytests
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
.PHONY: unit test verify go-lint update-gofmt verify-gofmt update-file-perms verify-file-perms update-spelling verify-spelling update-labels verify-labels
