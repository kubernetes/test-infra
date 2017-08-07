VERSION = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)
PROJECT ?= k8s-gubernator

# TODO(fejta): convert these to bazel rules

.PHONY: migrate versions deploy all
all: versions

github/webhook_secret:
	@echo TODO(kubernetes/test-infra/3884): deploy without needing this file
	@echo Ask Google GKE EngProd how to create this file.

secrets.json:
	@echo TODO(kubernetes/test-infra/3884): deploy without needing this file
	@echo Ask Google GKE EngProd how to create this file.

deploy: github/webhook_secret secrets.json
	gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet
	cd github && gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet

versions:
	gcloud app versions list --project="$(PROJECT)"

# TOOD(fejta): consider the following default:
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
