# Copyright 2017 The Kubernetes Authors.
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

VERSION := $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)
PROJECT ?= k8s-gubernator

# TODO(fejta): convert these to bazel rules

.PHONY: migrate versions deploy cron all
all: versions

deploy:
	gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet
	cd github && gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet

# This is a weird one. It's a global setting that's *not* versioned.
cron:
	gcloud app deploy --project="$(PROJECT)" cron.yaml

versions:
	gcloud app versions list --project="$(PROJECT)"

# TODO(fejta): consider the following default:
# gcloud app versions list --sort-by=~LAST_DEPLOYED --limit=1 --format='value(id)'
migrate: SHELL:=/bin/bash
migrate: versions
	@read -p "Promote $(SERVICE) [$(VERSION)]: " version && \
	gcloud app versions migrate "$${version:-$(VERSION)}" --project="$(PROJECT)" --quiet && \
	gcloud app versions migrate "$${version:-$(VERSION)}" --project="$(PROJECT)" --service=github --quiet


delete: SHELL:=/bin/bash
delete: versions
	@read -p "Delete $(SERVICE) [$(VERSION)]: " version && \
	gcloud app versions delete "$${version:-$(VERSION)}" --project="$(PROJECT)" && \
	gcloud app versions delete "$${version:-$(VERSION)}" --project="$(PROJECT)" --service=github
