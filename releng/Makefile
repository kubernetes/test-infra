# Copyright 2019 The Kubernetes Authors.
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

RELENG_BINARY_NAME?=releng

CONFIG_FORKER_BIN=../bazel-bin/releng/config-forker/config-forker_/config-forker
CONFIG_ROTATOR_BIN=../bazel-bin/releng/config-rotator/config-rotator_/config-rotator
GENERATE_TESTS_BIN=../bazel-bin/releng/generate_tests

YAML_CONFIG_PATH=test_config.yaml
OUTPUT_DIR=../config/jobs/kubernetes/generated/
TEST_GRID_OUTPUT_PATH=../config/testgrids/generated-test-config.yaml

all: build
# builds releng in a container, outputs to $(OUT_DIR)

# alias for building releng
build: releng

# Runs all tests
test: generate_tests_test
	echo "Ran Releng Tests!"

releng: build_generate_tests build_prepare_release_branch
	echo "Built releng targets!"

# Install local requirements
requirements:
	pip3 install ruamel.yaml sh

#
#  Building targets
#
GEN_TEST_OUT_PATH=../_bin-make/releng
GEN_TEST_BIN_NAME=$(addsuffix generate_tests,$(GEN_TEST_OUT_PATH)/)
build_generate_tests:
	if [ ! -d $(GEN_TEST_OUT_PATH) ]; then mkdir $(GEN_TEST_OUT_PATH); fi

	cat generate_tests.py > $(GEN_TEST_BIN_NAME)
	chmod a+x $(GEN_TEST_BIN_NAME)

PREP_REL_OUT_PATH=../_bin-make/releng
PREP_REL_BIN_NAME=$(addsuffix prepare_release_branch,$(PREP_REL_OUT_PATH)/)
build_prepare_release_branch:
	bazel build //releng/config-forker //releng/config-rotator //releng:generate_tests

	if [ ! -d $(PREP_REL_OUT_PATH) ]; then mkdir $(PREP_REL_OUT_PATH); fi

	cat prepare_release_branch.py > $(PREP_REL_BIN_NAME)
	chmod a+x $(PREP_REL_BIN_NAME)

#
#  Running targets
#
generate_tests:	
	python3 generate_tests.py --yaml-config-path $(YAML_CONFIG_PATH) --output-dir $(OUTPUT_DIR) --testgrid-output-path $(TEST_GRID_OUTPUT_PATH)

generate_tests_test:
	python3 generate_tests_test.py

prepare_release_branch: build_prepare_release_branch
	python3 prepare_release_branch.py $(CONFIG_ROTATOR_BIN) $(CONFIG_FORKER_BIN) $(GENERATE_TESTS_BIN)

.PHONY: all releng build prepare_release_branch build_generate_tests build_prepare_release_branch generate_tests generate_tests_test test
