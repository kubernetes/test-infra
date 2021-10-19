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

METRICS_BINARY_NAME?=metrics
OUT_PATH=../_bin-make/metrics
BIN_NAME=$(addsuffix bigquery,$(OUT_PATH)/)

OS=$(shell uname)
ifeq ($(shell uname),Darwin)
    JQ_PATH=/jq-1.5/jq-osx-amd64
else
    JQ_PATH=/jq-1.5/jq-linux64
endif

################################################################################
# ================================ Building ===================================
all: build

build: metrics

# Not including bigquery (broken)
metrics: bigquery-build
	echo "Built Metrics!"

bigquery-build:
	mkdir -p $(OUT_PATH)

	cat bigquery.py > $(BIN_NAME)
	chmod a+x $(BIN_NAME)

################################################################################
# ================================ Run ===================================
bigquery:
	./$(BIN_NAME)

################################################################################
# ================================ Test ===================================
bigquery-test:
	python3 bigquery_test.py --jq $(JQ_PATH)

test: bigquery-test
	echo "Ran Metrics Tests!"

################################################################################
# ================================ Clean ===================================
clean:
	rm -rf $(OUT_PATH)

.PHONY: all metrics build metrics test requirements bigquery bigquery-test clean
