# Copyright 2016 The Kubernetes Authors.
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

all: build test


HOOK_VERSION       = 0.98
SINKER_VERSION     = 0.6
DECK_VERSION       = 0.22
SPLICE_VERSION     = 0.17
MARQUE_VERSION     = 0.1
TOT_VERSION        = 0.0
CRIER_VERSION      = 0.5
HOROLOGIUM_VERSION = 0.3
PLANK_VERSION      = 0.9

# These are the usual GKE variables.
PROJECT ?= k8s-prow
ZONE ?= us-central1-f
CLUSTER ?= prow

update-config: get-cluster-credentials
	kubectl create configmap config --from-file=config=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -

update-plugins: get-cluster-credentials
	kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -

get-cluster-credentials:
	gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

build:
	go install ./cmd/...

test:
	go test -race -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: update-config update-plugins clean build test get-cluster-credentials

hook-image:
	CGO_ENABLED=0 go build -o cmd/hook/hook k8s.io/test-infra/prow/cmd/hook
	docker build -t "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)" cmd/hook
	gcloud docker -- push "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)"

hook-deployment:
	kubectl apply -f cluster/hook_deployment.yaml

hook-service:
	kubectl create -f cluster/hook_service.yaml

sinker-image:
	CGO_ENABLED=0 go build -o cmd/sinker/sinker k8s.io/test-infra/prow/cmd/sinker
	docker build -t "gcr.io/$(PROJECT)/sinker:$(SINKER_VERSION)" cmd/sinker
	gcloud docker -- push "gcr.io/$(PROJECT)/sinker:$(SINKER_VERSION)"

sinker-deployment:
	kubectl apply -f cluster/sinker_deployment.yaml

deck-image:
	CGO_ENABLED=0 go build -o cmd/deck/deck k8s.io/test-infra/prow/cmd/deck
	docker build -t "gcr.io/$(PROJECT)/deck:$(DECK_VERSION)" cmd/deck
	gcloud docker -- push "gcr.io/$(PROJECT)/deck:$(DECK_VERSION)"

deck-deployment:
	kubectl apply -f cluster/deck_deployment.yaml

deck-service:
	kubectl create -f cluster/deck_service.yaml

splice-image:
	CGO_ENABLED=0 go build -o cmd/splice/splice k8s.io/test-infra/prow/cmd/splice
	docker build -t "gcr.io/$(PROJECT)/splice:$(SPLICE_VERSION)" cmd/splice
	gcloud docker -- push "gcr.io/$(PROJECT)/splice:$(SPLICE_VERSION)"

splice-deployment:
	kubectl apply -f cluster/splice_deployment.yaml

marque-image:
	CGO_ENABLED=0 go build -o cmd/marque/marque k8s.io/test-infra/prow/cmd/marque
	docker build -t "gcr.io/$(PROJECT)/marque:$(MARQUE_VERSION)" cmd/marque
	gcloud docker -- push "gcr.io/$(PROJECT)/marque:$(MARQUE_VERSION)"

marque-deployment:
	kubectl apply -f cluster/marque_deployment.yaml

marque-service:
	kubectl apply -f cluster/marque_service.yaml

tot-image:
	CGO_ENABLED=0 go build -o cmd/tot/tot k8s.io/test-infra/prow/cmd/tot
	docker build -t "gcr.io/$(PROJECT)/tot:$(TOT_VERSION)" cmd/tot
	gcloud docker -- push "gcr.io/$(PROJECT)/tot:$(TOT_VERSION)"

tot-deployment:
	kubectl apply -f cluster/tot_deployment.yaml

tot-service:
	kubectl apply -f cluster/tot_service.yaml

crier-image:
	CGO_ENABLED=0 go build -o cmd/crier/crier k8s.io/test-infra/prow/cmd/crier
	docker build -t "gcr.io/$(PROJECT)/crier:$(CRIER_VERSION)" cmd/crier
	gcloud docker -- push "gcr.io/$(PROJECT)/crier:$(CRIER_VERSION)"

crier-deployment:
	kubectl apply -f cluster/crier_deployment.yaml

crier-service:
	kubectl apply -f cluster/crier_service.yaml

horologium-image:
	CGO_ENABLED=0 go build -o cmd/horologium/horologium k8s.io/test-infra/prow/cmd/horologium
	docker build -t "gcr.io/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)" cmd/horologium
	gcloud docker -- push "gcr.io/$(PROJECT)/horologium:$(HOROLOGIUM_VERSION)"

horologium-deployment:
	kubectl apply -f cluster/horologium_deployment.yaml

plank-image:
	CGO_ENABLED=0 go build -o cmd/plank/plank k8s.io/test-infra/prow/cmd/plank
	docker build -t "gcr.io/$(PROJECT)/plank:$(PLANK_VERSION)" cmd/plank
	gcloud docker -- push "gcr.io/$(PROJECT)/plank:$(PLANK_VERSION)"

plank-deployment:
	kubectl apply -f cluster/plank_deployment.yaml

.PHONY: hook-image hook-deployment hook-service test-pr-image sinker-image sinker-deployment deck-image deck-deployment deck-service splice-image splice-deployment marque-image marque-deployment marque-service tot-image tot-service tot-deployment crier-image crier-service crier-deployment horologium-image horologium-deployment plank-image plank-deployment
