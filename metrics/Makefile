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

include ../Makefile.base.mk
# Makefile.base.mk requires REPO_ROOT to be set
REPO_ROOT:=${CURDIR}/..

ensure-jq:
# ./get_jq.sh script returns the path of jq. The script downloads two versoins
# of jq, one is `jq-linux64` that's required to run in the container, the other
# one is `jq-<ARCH>` based on host OS, whis is copied over to `jq`. This target
# is only used for running in container, so make sure using the one that works
# in container.
	$(eval JQ_BIN=$(shell ./get_jq.sh)-linux64)

run: ensure-jq ensure-py-requirements3
	../hack/run-in-python-container.sh ./metrics/bigquery.py --jq=${JQ_BIN}

test-bigquery: ensure-jq ensure-py-requirements3
	../hack/run-in-python-container.sh ./metrics/bigquery_test.py --jq=${JQ_BIN}

test: test-bigquery

.PHONY: test
