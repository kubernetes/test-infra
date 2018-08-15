VERSION = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)
PROJECT ?= openshift-gce-devel

# TODO(fejta): convert these to bazel rules

.PHONY: migrate versions deploy all
all: versions

github/webhook_secret:
	cp ./../gubernator/github/webhook_secret ./github/

secrets.json:
	cp ./../gubernator/secrets.json ./

deploy: github/webhook_secret secrets.json
	gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet
	cd github && gcloud app deploy --version="$(VERSION)" --project="$(PROJECT)" --no-promote --quiet
	rm ./secrets.json github/webhook_secret

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
