KUBECONFIG ?= $(HOME)/.kube/config

push-nginx:
	# Replace the nginx config and restart the pod.
	kubectl --kubeconfig=$(KUBECONFIG) delete configmap nginx-https-redirect && \
	kubectl --kubeconfig=$(KUBECONFIG) create configmap nginx-https-redirect --from-file=nginx.conf=nginx-redirect/nginx.conf && \
	kubectl --kubeconfig=$(KUBECONFIG) delete pod -l app=nginx-https-redirect

push-ingress:
	# Update the ingress.
	kubectl --kubeconfig=$(KUBECONFIG) apply --record -f ingress.yaml

# Updates nginx https proxy and the ingress with the local nginx-redirect/nginx.conf and ingress.yaml files.
push-network: push-nginx push-ingress

targets := $(shell ls -d deployment/*/ | sed -e "s|deployment/\(.*\)/|\1|"| sed -e "s|^$$||")

update-all:
	@echo "Updating submit queues for repo(s) [$(targets)] in 4 seconds..."
	@sleep 4
	@$(foreach repo,$(targets),echo "Updating $(repo) submit-queue:"; APP=submit-queue TARGET=$(repo) make --no-print-directory -f ../Makefile push_config deploy;)

deploy-all:
	@echo "Deploying submit queues for repo(s) [$(targets)] in 4 seconds..."
	@sleep 4
	@$(foreach repo,$(targets),echo "Deploying $(repo) submit-queue:"; APP=submit-queue TARGET=$(repo) make --no-print-directory -f ../Makefile first_time_deploy;)

.PHONY: push-network update-all deploy-all push-nginx push-ingress
