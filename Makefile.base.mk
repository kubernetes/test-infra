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

# ${CURDIR} is the directory of Makefile that sources this file, so to ensure
# that the correct path is handled, Makefile sourcing this will need to set
# REPO_ROOT correctly to make it work
REPO_ROOT ?= ${CURDIR}

ensure-py-requirements3:
	${REPO_ROOT}/hack/run-in-python-container.sh pip3 install -r requirements3.txt
.PHONY: ensure-py-requirements3
