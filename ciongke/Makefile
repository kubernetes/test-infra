# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# These are the usual GKE variables. Project is purposefully empty for now.
PROJECT = k8s-ci-on-gke
ZONE = us-central1-b
NUM_NODES = 6
MACHINE_TYPE = n1-standard-8

# These are GitHub credentials.
# The hook secret is your HMAC token, the OAuth secret is the OAuth
# token of whatever account you want to comment and update statuses.
HOOK_SECRET_FILE = $(shell echo $${HOME}/hook)
OAUTH_SECRET_FILE = $(shell echo $${HOME}/k8s-oauth-token)
ORG = kubernetes
# This is test-infra-maintainers.
TEAM = 2009229
# If DRY_RUN is true then we won't make any mutating GitHub calls.
DRY_RUN = true

SOURCE_BUCKET = k8s-ci-on-gke-source

HOOK_VERSION = 0.0
TEST_PR_VERSION = 0.0
RUN_TEST_VERSION = 0.0

# Useful rules:
# - create-cluster turns up a cluster then prints out the webhook address.
# - update-cluster pushes new image versions and updates the deployment.
# - delete-cluster deletes the cluster.
# - hook-image builds and pushes the latest hook image.
# - hook-deployment updates the deployment.
# - hook-service create the hook service.

create-cluster:
	gcloud -q container --project "$(PROJECT)" clusters create ciongke --zone "$(ZONE)" --machine-type "$(MACHINE_TYPE)" --scope "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --num-nodes "$(NUM_NODES)" --network "default" --enable-cloud-logging --enable-cloud-monitoring
	gsutil mb -p $(PROJECT) gs://$(SOURCE_BUCKET)
	gsutil acl ch -u AllUsers:R gs://$(SOURCE_BUCKET)
	gsutil defacl ch -u AllUsers:R gs://$(SOURCE_BUCKET)
	kubectl create secret generic hmac-token --from-file=hmac=$(HOOK_SECRET_FILE)
	kubectl create secret generic oauth-token --from-file=oauth=$(OAUTH_SECRET_FILE)
	make test-pr-image --no-print-directory
	make hook-image --no-print-directory
	make hook-deployment --no-print-directory
	make hook-service --no-print-directory
	@echo -n "Waiting for loadbalancer ingress "; while [[ "$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip)" == "null" ]]; do echo -n "."; sleep 5; done; echo " Done"
	@echo "Webhook address: http://$$(kubectl get svc hook -o=json | jq -r .status.loadBalancer.ingress[0].ip):8888/"

update-cluster:
	make run-test-image --no-print-directory
	make test-pr-image --no-print-directory
	make hook-image --no-print-directory
	make hook-deployment --no-print-directory

delete-cluster:
	@echo "Deleting cluster in $(PROJECT) in 5 seconds..."; sleep 5
	gcloud -q container --project "$(PROJECT)" clusters delete ciongke --zone "$(ZONE)"

clean:
	rm cmd/hook/hook cmd/test-pr/test-pr

test:
	go test -cover $$(go list ./... | grep -v "\/vendor\/")

.PHONY: create-cluster delete-cluster clean test

hook-image:
	CGO_ENABLED=0 go build -o cmd/hook/hook github.com/kubernetes/test-infra/ciongke/cmd/hook
	docker build -t "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)" cmd/hook
	gcloud docker push "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)"

hook-deployment:
	@echo "$${hook_deployment}" | kubectl apply -f -

hook-service:
	@echo "$${hook_service}" | kubectl create -f -

test-pr-image:
	CGO_ENABLED=0 go build -o cmd/test-pr/test-pr github.com/kubernetes/test-infra/ciongke/cmd/test-pr
	docker build -t "gcr.io/$(PROJECT)/test-pr:$(TEST_PR_VERSION)" cmd/test-pr
	gcloud docker push "gcr.io/$(PROJECT)/test-pr:$(TEST_PR_VERSION)"

run-test-image:
	CGO_ENABLED=0 go build -o cmd/run-test/run-test github.com/kubernetes/test-infra/ciongke/cmd/run-test
	docker build -t "gcr.io/$(PROJECT)/run-test:$(RUN_TEST_VERSION)" cmd/run-test
	gcloud docker push "gcr.io/$(PROJECT)/run-test:$(RUN_TEST_VERSION)"

.PHONY: hook-image hook-deployment hook-service test-pr-image run-test-image

# We need to replace project and version and so on in these YAML definitions,
# and there's not really a great way to do that.
define hook_deployment
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: hook
  labels:
    app: hook
spec:
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 1
  template:
    metadata:
      labels:
        app: hook
    spec:
      terminationGracePeriodSeconds: 30
      containers:
      - name: hook
        image: "gcr.io/$(PROJECT)/hook:$(HOOK_VERSION)"
        imagePullPolicy: Always
        args: 
        - -run-test-image=gcr.io/$(PROJECT)/run-test:$(RUN_TEST_VERSION)
        - -test-pr-image=gcr.io/$(PROJECT)/test-pr:$(TEST_PR_VERSION)
        - -source-bucket=$(SOURCE_BUCKET)
        - -org=$(ORG)
        - -team=$(TEAM)
        - -dry-run=$(DRY_RUN)
        ports:
          - name: http
            containerPort: 8888
        resources:
          limits:
            cpu: 1.0
            memory: 1Gi
        volumeMounts:
        - name: hmac
          mountPath: /etc/hmac
          readOnly: true
        - name: oauth
          mountPath: /etc/oauth
          readOnly: true
      volumes:
      - name: hmac
        secret:
          secretName: hmac-token
      - name: oauth
        secret:
          secretName: oauth-token
endef
define hook_service
apiVersion: v1
kind: Service
metadata:
  name: hook
spec:
  selector:
    app: hook
  ports:
  - port: 8888
  type: LoadBalancer
endef
export hook_deployment
export hook_service
