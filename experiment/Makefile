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

# ARGS can be overridden by calling make
ARGS ?= ""

run-compare-yaml: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/compare-yaml.py ${ARGS}
.PHONY: run-compare-yaml

run-edit-prowjobs: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/edit-prowjobs.py ${ARGS}
.PHONY: run-edit-prowjobs

run-find_developers: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/find_developers.py ${ARGS}
.PHONY: run-find_developers

run-find-merge-conflicts: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/find-merge-conflicts.py ${ARGS}
.PHONY: run-find-merge-conflicts

run-get_job_pods: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/get_job_pods.py ${ARGS}
.PHONY: run-get_job_pods

run-flakedetector: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/flakedetector.py ${ARGS}
.PHONY: run-flakedetector

run-graphql_issue_example: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/graphql_issue_example.py ${ARGS}
.PHONY: run-flakedetector

run-migrate_testgrid_tabs: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/migrate_testgrid_tabs.py ${ARGS}
.PHONY: run-migrate_testgrid_tabs

run-move-annotations: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/move-annotations.py ${ARGS}
.PHONY: run-move-annotations

run-parse_build_log: ensure-py-requirements3
	../hack/run-in-python-container.sh ./experiment/parse_build_log.py ${ARGS}
.PHONY: run-parse_build_log
