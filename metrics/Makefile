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
    JQ_PATH=jq-1.5/jq-osx-amd64
else
    JQ_PATH=jq-1.5/jq-linux64
endif

all: build

build: metrics

# Not including bigquery (broken)
metrics: bigquery
	echo "Built Metrics!"

test: bigquery_test
	echo "Ran Metrics Tests!"

requirements:
	pip3 install certifi chardet idna python-dateutil pytz ruamel.yaml requests six urllib3	

bigquery:
	if [ ! -d $(OUT_PATH) ]; then mkdir $(OUT_PATH); fi

	cat bigquery.py > $(BIN_NAME)
	chmod a+x $(BIN_NAME)

bigquery-test:
	if [ ! -d "jq-1.5" ]; then ./get_jq.sh; fi
	python3 bigquery_test.py --jq $(JQ_PATH)

clean:
	rm -rf jq-1.5

.PHONY: all metrics build metrics test requirements bigquery bigquery-test clean
