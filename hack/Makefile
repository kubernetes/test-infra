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

HACK_BINARY_NAME?=hack

OUT_PATH=../_bin-make/hack
BIN_NAME=$(addsuffix pylint_bin,$(OUT_PATH)/)

all: build

build: hack

hack: pylint

pylint:
	if [ ! -d $(OUT_PATH) ]; then mkdir $(OUT_PATH); fi

	cat pylint_bin.py > $(BIN_NAME)
	chmod a+x $(BIN_NAME)

test:
	python3 coalesce_test.py

.PHONY: all hack build pylint test