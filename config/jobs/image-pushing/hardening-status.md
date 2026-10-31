<!--
Regenerate from the repository root with Ruby and its standard YAML library.
Only sibling YAML files are included; periodic jobs use extra_refs for the repo.
Security status follows the project and service-account rules, not a full security audit.
Everything through the AUTOGEN marker is preserved; content below it is replaced.

ruby -ryaml <<'RUBY'
folder = "config/jobs/image-pushing"
path = "#{folder}/hardening-status.md"
rows = []
Dir["#{folder}/*.{yaml,yml}"].sort.each do |source|
	YAML.load_file(source).each do |kind, entries|
		raise "Unexpected job category: #{kind}" unless %w[presubmits postsubmits periodics].include?(kind)
		groups = kind == "periodics" ? {nil => entries} : entries
		groups.each do |repo, jobs|
			jobs.each do |job|
				slug = repo || job.fetch("extra_refs").map { |ref| "#{ref.fetch("org")}/#{ref.fetch("repo")}" }.join(", ")
				spec = job.fetch("spec")
				account = spec.fetch("serviceAccountName")
				projects = spec.fetch("containers").flat_map do |container|
					args = container.fetch("command", []) + container.fetch("args", [])
					args.each_with_index.map do |arg, index|
						arg.start_with?("--project=") ? arg.split("=", 2).last : (args[index + 1] if arg == "--project")
					end.compact
				end.uniq
				raise "Expected one project for #{job.fetch("name")}" unless projects.length == 1
				project = projects.first
				status = if account != "gcb-builder"
					"🟢"
				elsif project == "k8s-staging-images"
					"🟠"
				else
					"🔴"
				end
				cells = [job.fetch("name"), slug, (project == "k8s-staging-images").to_s, account, project, status]
				rows << "| #{cells.join(" | ")} |"
			end
		end
	end
end
lines = File.readlines(path)
marker = "<!-- AUTOGEN --" + ">"
marker_end = lines.index { |line| line.chomp == marker }
raise "AUTOGEN marker not found" unless marker_end
headers = [
	"| Job Name  | Repository Slug  | Is the job using `k8s-staging-images` project? | Configured Kubernetes Service Account Name  | Configured Project | Is the job configured securely? |",
	"| --- | --- | --- | --- | --- | --- |"
]
prefix = lines.take(marker_end + 1).join
prefix += "\n" unless prefix.end_with?("\n")
File.write(path, prefix + (headers + rows).join("\n") + "\n")
RUBY
-->

# Tracking the k8s-staging-images migration and switching to secure image build pipelines.

We are migrating all our image builds for all repositories to a shared GCP project called `k8s-staging-images`. Google released Artifact Registry which enables multiple image registries per project with their own RBAC. Right now, our image pipeline shares a single service account called gcb-builder so any job can write all the registries and buckets the service account has access to.

Going forward, our new approach is as follows:
- Every GitHub repository will have their own registry in the format `us-central1-docker.pkg.dev/k8s-staging-images/[REGISTRY]` along with a dedicated Google Service Account that can only write to their own image registry.
- In the k8s.io repository, when you provision a registry, a dedicated Kubernetes Service Account will be created that will be tied to 
- gcb-builder will be deprecated and removed.

## How to mark you project as green?

1. If the configured project is set to k8s-staging-images, then all you need to do is the following:
  - set the Kubernetes Service Account of your job to the one matching your registry. They are listed [here](https://github.com/kubernetes/k8s.io/blob/main/kubernetes/gke-prow-build-trusted/prow/builds-sa.yaml). You'll also need to set resource requests/limits which should be 0.5 cores and 1Gi of memory.
  - Amend your cloudbuild job 
2. If you your job is marked as 🔴, then you need to do the following:
  - create a staging registry as described [here](https://github.com/kubernetes/k8s.io/tree/main/registry.k8s.io).
  - amend your build job to use the new staging registry.
  - do all the steps in 1 (they are documented in the staging registry docs)
3. The following projects are special cases and shouldn't be remediated at this time:
   - k8s-staging-test-infra
   - k8s-staging-kubetest2
   - k8s-staging-e2e-test-images
   - k8s-staging-sig-storage

## Status

<!-- AUTOGEN -->
| Job Name  | Repository Slug  | Is the job using `k8s-staging-images` project? | Configured Kubernetes Service Account Name  | Configured Project | Is the job configured securely? |
| --- | --- | --- | --- | --- | --- |
| post-k8s-infra-prow-images | kubernetes-sigs/prow | false | gcb-builder | k8s-infra-prow | 🔴 |
| post-agent-sandbox-push-images | kubernetes-sigs/agent-sandbox | true | gcb-builder | k8s-staging-images | 🟠 |
| post-agentic-net-push-controller-image | kubernetes-sigs/kube-agentic-networking | true | gcb-builder | k8s-staging-images | 🟠 |
| post-agentic-net-push-adk-agent-image | kubernetes-sigs/kube-agentic-networking | true | gcb-builder | k8s-staging-images | 🟠 |
| post-agentic-net-push-everything-mcp-image | kubernetes-sigs/kube-agentic-networking | true | gcb-builder | k8s-staging-images | 🟠 |
| post-agentic-net-push-langchain-agent-image | kubernetes-sigs/kube-agentic-networking | true | gcb-builder | k8s-staging-images | 🟠 |
| post-agentic-net-push-openai-agent-image | kubernetes-sigs/kube-agentic-networking | true | gcb-builder | k8s-staging-images | 🟠 |
| apiserver-network-proxy-push-images | kubernetes-sigs/apiserver-network-proxy | false | gcb-builder | k8s-staging-kas-network-proxy | 🔴 |
| apisnoop-push-auditlogger-images | kubernetes-sigs/apisnoop | false | gcb-builder | k8s-staging-apisnoop | 🔴 |
| apisnoop-push-snoopdb-images | kubernetes-sigs/apisnoop | false | gcb-builder | k8s-staging-apisnoop | 🔴 |
| post-verify-conformance-push-images | kubernetes-sigs/verify-conformance | false | gcb-builder | k8s-staging-apisnoop | 🔴 |
| post-autoscaler-push-addon-resizer-images | kubernetes/autoscaler | false | gcb-builder | k8s-staging-autoscaling | 🔴 |
| post-autoscaler-push-vpa-images | kubernetes/autoscaler | false | gcb-builder | k8s-staging-autoscaling | 🔴 |
| post-autoscaler-push-cluster-autoscaler-images | kubernetes/autoscaler | false | gcb-builder | k8s-staging-autoscaling | 🔴 |
| post-bom-push-images | kubernetes-sigs/bom | false | gcb-builder | k8s-staging-bom | 🔴 |
| post-boskos-push-images | kubernetes-sigs/boskos | false | gcb-builder | k8s-staging-boskos | 🔴 |
| cloud-provider-aws-push-images | kubernetes/cloud-provider-aws | false | gcb-builder | k8s-staging-provider-aws | 🔴 |
| aws-ebs-csi-driver-push-images | kubernetes-sigs/aws-ebs-csi-driver | false | gcb-builder | k8s-staging-provider-aws | 🔴 |
| aws-iam-authenticator-push-images | kubernetes-sigs/aws-iam-authenticator | false | gcb-builder | k8s-staging-provider-aws | 🔴 |
| aws-encryption-provider-push-images | kubernetes-sigs/aws-encryption-provider | false | gcb-builder | k8s-staging-provider-aws | 🔴 |
| post-gcp-filestore-push-images | kubernetes-sigs/gcp-filestore-csi-driver | false | gcb-builder | k8s-staging-cloud-provider-gcp | 🔴 |
| post-gcp-compute-persistent-disk-csi-driver-push-images | kubernetes-sigs/gcp-compute-persistent-disk-csi-driver | false | gcb-builder | k8s-staging-cloud-provider-gcp | 🔴 |
| post-cloud-provider-gcp-push-images | kubernetes/cloud-provider-gcp | false | gcb-builder | k8s-staging-cloud-provider-gcp | 🔴 |
| post-ibm-vpc-block-csi-driver-push-images | kubernetes-sigs/ibm-vpc-block-csi-driver | false | gcb-builder | k8s-staging-cloud-provider-ibm | 🔴 |
| post-ibm-powervs-block-csi-driver-push-images | kubernetes-sigs/ibm-powervs-block-csi-driver | false | gcb-builder | k8s-staging-cloud-provider-ibm | 🔴 |
| post-cloud-provider-kind-image | kubernetes-sigs/cloud-provider-kind | true | gcb-builder | k8s-staging-images | 🟠 |
| post-cloud-provider-vsphere-push-images | kubernetes/cloud-provider-vsphere | false | gcb-builder | k8s-staging-cloud-pv-vsphere | 🔴 |
| cloud-provider-vsphere-push-images-nightly | kubernetes/cloud-provider-vsphere | false | gcb-builder | k8s-staging-cloud-pv-vsphere | 🔴 |
| cluster-addons-postsubmit-push-to-staging | kubernetes-sigs/cluster-addons | false | gcb-builder | k8s-staging-cluster-addons | 🔴 |
| post-cluster-api-push-images | kubernetes-sigs/cluster-api | false | gcb-builder | k8s-staging-cluster-api | 🔴 |
| post-cluster-api-provider-vsphere-push-images | kubernetes-sigs/cluster-api-provider-vsphere | false | gcb-builder | k8s-staging-capi-vsphere | 🔴 |
| post-cluster-api-addon-provider-helm-push-images | kubernetes-sigs/cluster-api-addon-provider-helm | false | gcb-builder | k8s-staging-cluster-api-helm | 🔴 |
| post-cluster-api-provider-aws-push-images | kubernetes-sigs/cluster-api-provider-aws | false | gcb-builder | k8s-staging-cluster-api-aws | 🔴 |
| post-cluster-api-provider-azure-push-images | kubernetes-sigs/cluster-api-provider-azure | false | gcb-builder | k8s-staging-cluster-api-azure | 🔴 |
| post-cluster-api-provider-cloudstack-push-images | kubernetes-sigs/cluster-api-provider-cloudstack | false | gcb-builder | k8s-staging-capi-cloudstack | 🔴 |
| post-cluster-api-provider-digitalocean-push-images | kubernetes-sigs/cluster-api-provider-digitalocean | false | gcb-builder | k8s-staging-cluster-api-do | 🔴 |
| post-cluster-api-provider-gcp-push-images | kubernetes-sigs/cluster-api-provider-gcp | false | gcb-builder | k8s-staging-cluster-api-gcp | 🔴 |
| post-cluster-api-provider-openstack-push-images | kubernetes-sigs/cluster-api-provider-openstack | false | gcb-builder | k8s-staging-capi-openstack | 🔴 |
| post-cluster-api-operator-push-images | kubernetes-sigs/cluster-api-operator | false | gcb-builder | k8s-staging-capi-operator | 🔴 |
| post-image-builder-push-images | kubernetes-sigs/image-builder | false | gcb-builder | k8s-staging-scl-image-builder | 🔴 |
| post-cluster-api-provider-ibmcloud-push-images | kubernetes-sigs/cluster-api-provider-ibmcloud | false | gcb-builder | k8s-staging-capi-ibmcloud | 🔴 |
| post-ibm-powervs-cloud-provider-push-images | kubernetes-sigs/cluster-api-provider-ibmcloud | false | gcb-builder | k8s-staging-capi-ibmcloud | 🔴 |
| post-cluster-api-ipam-provider-in-cluster | kubernetes-sigs/cluster-api-ipam-provider-in-cluster | false | gcb-builder | k8s-staging-capi-ipam-ic | 🔴 |
| cluster-api-push-images-nightly | kubernetes-sigs/cluster-api | false | gcb-builder | k8s-staging-cluster-api | 🔴 |
| cluster-api-provider-vsphere-push-images-nightly | kubernetes-sigs/cluster-api-provider-vsphere | false | gcb-builder | k8s-staging-capi-vsphere | 🔴 |
| cluster-api-provider-aws-push-images-nightly | kubernetes-sigs/cluster-api-provider-aws | false | gcb-builder | k8s-staging-cluster-api-aws | 🔴 |
| cluster-api-provider-gcp-push-images-nightly | kubernetes-sigs/cluster-api-provider-gcp | false | gcb-builder | k8s-staging-cluster-api-gcp | 🔴 |
| post-cluster-capacity-push-images | kubernetes-sigs/cluster-capacity | false | gcb-builder | k8s-staging-cluster-capacity | 🔴 |
| post-cluster-inventory-api-push-images | kubernetes-sigs/cluster-inventory-api | true | gcb-builder | k8s-staging-images | 🟠 |
| post-contributor-site-push-image-k8s-contrib-site-hugo | kubernetes/contributor-site | true | gcb-builder | k8s-staging-images | 🟠 |
| post-cri-tools-images | kubernetes-sigs/cri-tools | false | gcb-builder | k8s-staging-cri-tools | 🔴 |
| secrets-store-csi-driver-push-image | kubernetes-sigs/secrets-store-csi-driver | false | gcb-builder | k8s-staging-csi-secrets-store | 🔴 |
| post-vsphere-csi-driver-push-images | kubernetes-sigs/vsphere-csi-driver | true | gcb-builder | k8s-staging-images | 🟠 |
| post-descheduler-push-images | kubernetes-sigs/descheduler | false | gcb-builder | k8s-staging-descheduler | 🔴 |
| dns-push-images | kubernetes/dns | false | gcb-builder | k8s-staging-dns | 🔴 |
| node-local-dns-push-images | kubernetes-sigs/node-local-dns | true | gcb-builder | k8s-staging-images | 🟠 |
| post-dra-driver-cpu-push-images | kubernetes-sigs/dra-driver-cpu | true | gcb-builder | k8s-staging-images | 🟠 |
| post-dra-driver-google-tpu-push-images | kubernetes-sigs/dra-driver-google-tpu | true | gcb-builder | k8s-staging-images | 🟠 |
| post-dra-driver-nvidia-gpu-push-images | kubernetes-sigs/dra-driver-nvidia-gpu | true | gcb-builder | k8s-staging-images | 🟠 |
| post-dra-example-driver-image | kubernetes-sigs/dra-example-driver | true | gcb-builder | k8s-staging-images | 🟠 |
| post-dranet-image | kubernetes-sigs/dranet | false | gcb-builder | k8s-staging-networking | 🔴 |
| post-kubernetes-push-e2e-agnhost-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-apparmor-loader-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-busybox-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-glibc-dns-testing-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-ipc-utils-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-kitten-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-nautilus-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-nginx-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-nginx-new-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-node-perf-npb-ep-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-node-perf-npb-is-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-node-perf-pytorch-wide-deep-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-nonroot-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-pets-peer-finder-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-pets-zookeeper-installer-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-regression-issue-74839-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-resource-consumer-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-sample-apiserver-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-sample-device-plugin-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-volume-iscsi-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-volume-nfs-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| post-kubernetes-push-e2e-windows-servercore-cache-test-images | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| kubernetes-e2e-windows-servercore-cache | kubernetes/kubernetes | false | gcb-builder | k8s-staging-e2e-test-images | 🔴 |
| etcd-manager-postsubmit-push-to-staging | kubernetes-sigs/etcd-manager | true | gcb-builder | k8s-staging-images | 🟠 |
| post-external-dns-push-images | kubernetes-sigs/external-dns | false | gcb-builder | k8s-staging-external-dns | 🔴 |
| post-gateway-api-conformance-images-push-images | kubernetes-sigs/gateway-api-conformance-images | true | gcb-builder | k8s-staging-images | 🟠 |
| post-windows-gmsa-push-images | kubernetes-sigs/windows-gmsa | false | gcb-builder | k8s-staging-gmsa-webhook | 🔴 |
| post-headlamp-push-images | kubernetes-sigs/headlamp | true | gcb-builder | k8s-staging-images | 🟠 |
| post-inference-extension-push-images | kubernetes-sigs/gateway-api-inference-extension | true | gcb-builder | k8s-staging-images | 🟠 |
| post-inference-perf-push-images | kubernetes-sigs/inference-perf | true | gcb-builder | k8s-staging-images | 🟠 |
| post-registry-push-images | kubernetes/registry.k8s.io | true | infra-tools | k8s-staging-images | 🟢 |
| post-porche-push-images | kubernetes-sigs/porche | true | gcb-builder | k8s-staging-images | 🟠 |
| post-k8s-deploy-registry | kubernetes/k8s.io | true | infra-tools | k8s-staging-images | 🟢 |
| post-k8sio-push-image-octodns-docker | kubernetes/k8s.io | true | infra-tools | k8s-staging-images | 🟢 |
| post-k8sio-push-image-k8s-infra | kubernetes/k8s.io | false | gcb-builder | k8s-staging-infra-tools | 🔴 |
| post-k8sio-push-image-public-log-asn-matcher | kubernetes/k8s.io | false | gcb-builder | k8s-staging-infra-tools | 🔴 |
| post-k8sio-push-image-cs-fetch-repos-canary | kubernetes/k8s.io | false | gcb-builder | k8s-staging-infra-tools | 🔴 |
| post-ingress-conformance-push-echoserver-image | kubernetes-sigs/ingress-controller-conformance | false | gcb-builder | k8s-staging-ingressconformance | 🔴 |
| post-ingress-conformance-push-reports-image | kubernetes-sigs/ingress-controller-conformance | false | gcb-builder | k8s-staging-ingressconformance | 🔴 |
| post-ingress-conformance-push-image | kubernetes-sigs/ingress-controller-conformance | false | gcb-builder | k8s-staging-ingressconformance | 🔴 |
| post-ingress-gce-push-image | kubernetes/ingress-gce | false | gcb-builder | k8s-ingress-image-push | 🔴 |
| post-jobset-push-images | kubernetes-sigs/jobset | true | gcb-builder | k8s-staging-images | 🟠 |
| post-karpenter-provider-cluster-api-push-images | kubernetes-sigs/karpenter-provider-cluster-api | true | gcb-builder | k8s-staging-images | 🟠 |
| post-kind-push-binaries | kubernetes-sigs/kind | false | gcb-builder | k8s-staging-kind | 🔴 |
| post-kind-push-base-image | kubernetes-sigs/kind | false | gcb-builder | k8s-staging-kind | 🔴 |
| post-kind-push-kindnetd-image | kubernetes-sigs/kind | false | gcb-builder | k8s-staging-kind | 🔴 |
| post-kind-push-local-path-provisioner-image | kubernetes-sigs/kind | false | gcb-builder | k8s-staging-kind | 🔴 |
| post-kind-push-local-path-helper-image | kubernetes-sigs/kind | false | gcb-builder | k8s-staging-kind | 🔴 |
| post-kindnet-image | kubernetes-sigs/kindnet | false | gcb-builder | k8s-staging-networking | 🔴 |
| post-kernel-module-management-push-images | kubernetes-sigs/kernel-module-management | false | gcb-builder | k8s-staging-kmm | 🔴 |
| kops-postsubmit-push-to-staging | kubernetes/kops | false | gcb-builder | k8s-staging-kops | 🔴 |
| kro-push-images | kubernetes-sigs/kro | true | gcb-builder | k8s-staging-images | 🟠 |
| post-kube-network-policies-image | kubernetes-sigs/kube-network-policies | false | gcb-builder | k8s-staging-networking | 🔴 |
| post-kube-state-metrics-push-images | kubernetes/kube-state-metrics | false | gcb-builder | k8s-staging-kube-state-metrics | 🔴 |
| post-kube-rbac-proxy-push-images | kubernetes-sigs/kubebuilder | false | gcb-builder | k8s-staging-kubebuilder | 🔴 |
| post-kubetest2-push-binaries | kubernetes-sigs/kubetest2 | false | gcb-builder | k8s-staging-kubetest2 | 🔴 |
| ci-kubetest2-push-binaries | kubernetes-sigs/kubetest2 | false | gcb-builder | k8s-staging-kubetest2 | 🔴 |
| post-kueue-push-images | kubernetes-sigs/kueue | true | gcb-builder | k8s-staging-images | 🟠 |
| periodic-kueue-push-images | kubernetes-sigs/kueue | true | gcb-builder | k8s-staging-images | 🟠 |
| post-kustomize-push-images | kubernetes-sigs/kustomize | false | gcb-builder | k8s-staging-kustomize | 🔴 |
| post-kwok-push-images | kubernetes-sigs/kwok | false | gcb-builder | k8s-staging-kwok | 🔴 |
| post-lws-push-images | kubernetes-sigs/lws | true | gcb-builder | k8s-staging-images | 🟠 |
| post-maintainer-tools-push-antigravity-image | kubernetes-sigs/maintainer-tools | false | gcb-builder | k8s-staging-infra-tools | 🔴 |
| periodic-maintainer-tools-push-antigravity-image | kubernetes-sigs/maintainer-tools | false | gcb-builder | k8s-staging-infra-tools | 🔴 |
| post-mcp-lifecycle-operator-push-images | kubernetes-sigs/mcp-lifecycle-operator | true | gcb-builder | k8s-staging-images | 🟠 |
| post-metrics-server-push-images | kubernetes-sigs/metrics-server | false | gcb-builder | k8s-staging-metrics-server | 🔴 |
| post-minikube-kubernetes-bootcamp-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-gvisor-addon-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-kubernetes-registry-proxy-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-kicbase-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-buildroot-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-storage-provisioner-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-minikube-auto-pause-hook-image | kubernetes/minikube | true | gcb-builder | k8s-staging-images | 🟠 |
| post-nat64-image | kubernetes-sigs/nat64 | false | gcb-builder | k8s-staging-networking | 🔴 |
| post-node-feature-discovery-operator-push-images | kubernetes-sigs/node-feature-discovery-operator | false | gcb-builder | k8s-staging-nfd | 🔴 |
| post-node-feature-discovery-push-images | kubernetes-sigs/node-feature-discovery | false | gcb-builder | k8s-staging-nfd | 🔴 |
| post-node-ipam-controller-push-images | kubernetes-sigs/node-ipam-controller | false | gcb-builder | k8s-staging-networking | 🔴 |
| node-problem-detector-push-images | kubernetes/node-problem-detector | false | gcb-builder | k8s-staging-npd | 🔴 |
| post-node-readiness-controller-push-images | kubernetes-sigs/node-readiness-controller | true | gcb-builder | k8s-staging-images | 🟠 |
| post-kubernetes-push-perf-tests-access-tokens | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-containerd | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-probes | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-request-benchmark | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-scratch | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-sleep | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-kubernetes-push-perf-tests-watch-list | kubernetes/perf-tests | false | gcb-builder | k8s-staging-perf-tests | 🔴 |
| post-prometheus-adapter-push-images | kubernetes-sigs/prometheus-adapter | false | gcb-builder | k8s-staging-prometheus-adapter | 🔴 |
| post-provider-azure-push-images | kubernetes-sigs/cloud-provider-azure | false | gcb-builder | k8s-staging-provider-azure | 🔴 |
| cloud-provider-openstack-push-images | kubernetes/cloud-provider-openstack | false | gcb-builder | k8s-staging-provider-os | 🔴 |
| post-kube-scheduler-simulator-push-images | kubernetes-sigs/kube-scheduler-simulator | false | gcb-builder | k8s-staging-sched-simulator | 🔴 |
| post-scheduler-plugins-push-images | kubernetes-sigs/scheduler-plugins | false | gcb-builder | k8s-staging-scheduler-plugins | 🔴 |
| secrets-store-sync-controller-push-image | kubernetes-sigs/secrets-store-sync-controller | true | gcb-builder | k8s-staging-images | 🟠 |
| post-security-profiles-operator-push-image | kubernetes-sigs/security-profiles-operator | true | sp-operator | k8s-staging-images | 🟢 |
| post-website-push-image-k8s-website-hugo | kubernetes/website | false | gcb-builder | k8s-staging-sig-docs | 🔴 |
| post-csi-driver-host-path-push-images | kubernetes-csi/csi-driver-host-path | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-csi-driver-smb-push-images | kubernetes-csi/csi-driver-smb | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-csi-test-push-images | kubernetes-csi/csi-test | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-attacher-push-images | kubernetes-csi/external-attacher | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-health-monitor-push-images | kubernetes-csi/external-health-monitor | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-provisioner-push-images | kubernetes-csi/external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-resizer-push-images | kubernetes-csi/external-resizer | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-snapshotter-push-images | kubernetes-csi/external-snapshotter | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-external-snapshot-metadata-push-images | kubernetes-csi/external-snapshot-metadata | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-livenessprobe-push-images | kubernetes-csi/livenessprobe | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-node-driver-registrar-push-images | kubernetes-csi/node-driver-registrar | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-csi-driver-nfs-push-images | kubernetes-csi/csi-driver-nfs | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-csi-driver-iscsi-push-images | kubernetes-csi/csi-driver-iscsi | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-lib-volume-populator-push-images | kubernetes-csi/lib-volume-populator | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-volume-data-source-validator-push-images | kubernetes-csi/volume-data-source-validator | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-sig-storage-local-static-provisioner-push-images | kubernetes-sigs/sig-storage-local-static-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-nfs-ganesha-server-and-external-provisioner-push-images | kubernetes-sigs/nfs-ganesha-server-and-external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-nfs-subdir-external-provisioner-push-images | kubernetes-sigs/nfs-subdir-external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-container-object-storage-interface-push-images | kubernetes-sigs/container-object-storage-interface | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-cosi-driver-sample-push-images | kubernetes-sigs/cosi-driver-sample | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-csi-proxy-push-images | kubernetes-csi/csi-proxy | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-csi-driver-host-path-push-images | kubernetes-csi/csi-driver-host-path | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-csi-driver-smb-push-images | kubernetes-csi/csi-driver-smb | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-csi-test-push-images | kubernetes-csi/csi-test | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-attacher-push-images | kubernetes-csi/external-attacher | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-health-monitor-push-images | kubernetes-csi/external-health-monitor | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-provisioner-push-images | kubernetes-csi/external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-resizer-push-images | kubernetes-csi/external-resizer | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-snapshotter-push-images | kubernetes-csi/external-snapshotter | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-external-snapshot-metadata-push-images | kubernetes-csi/external-snapshot-metadata | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-livenessprobe-push-images | kubernetes-csi/livenessprobe | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-node-driver-registrar-push-images | kubernetes-csi/node-driver-registrar | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-csi-driver-nfs-push-images | kubernetes-csi/csi-driver-nfs | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-csi-driver-iscsi-push-images | kubernetes-csi/csi-driver-iscsi | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-lib-volume-populator-push-images | kubernetes-csi/lib-volume-populator | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-volume-data-source-validator-push-images | kubernetes-csi/volume-data-source-validator | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-sig-storage-local-static-provisioner-push-images | kubernetes-sigs/sig-storage-local-static-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-nfs-ganesha-server-and-external-provisioner-push-images | kubernetes-sigs/nfs-ganesha-server-and-external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-nfs-subdir-external-provisioner-push-images | kubernetes-sigs/nfs-subdir-external-provisioner | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-container-object-storage-interface-push-images | kubernetes-sigs/container-object-storage-interface | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| canary-cosi-driver-sample-push-images | kubernetes-sigs/cosi-driver-sample | false | gcb-builder | k8s-staging-sig-storage | 🔴 |
| post-slack-infra-push-images | kubernetes-sigs/slack-infra | false | gcb-builder | k8s-staging-slack-infra | 🔴 |
| kube-storage-version-migrator-push-images | kubernetes-sigs/kube-storage-version-migrator | false | gcb-builder | k8s-staging-storage-migrator | 🔴 |
| post-tejolote-push-images | kubernetes-sigs/tejolote | false | gcb-builder | k8s-staging-tejolote | 🔴 |
| post-test-infra-push-bazelbuild | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-benchmarkjunit | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-bigquery | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-bootstrap | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-gcloud-in-go | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-image-builder | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-krte | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-kubekins-e2e | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-kubekins-e2e-v2 | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-alpine | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-git | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-git-custom-k8s-auth | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-misc-images | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-kettle | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-test-infra-push-triage | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| periodic-test-infra-push-krte | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| periodic-test-infra-push-kubekins-e2e | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| periodic-test-infra-push-kubekins-e2e-v2 | kubernetes/test-infra | false | gcb-builder | k8s-staging-test-infra | 🔴 |
| post-testgrid-json-exporter-push-image | kubernetes-sigs/testgrid-json-exporter | false | gcb-builder | k8s-staging-tg-exporter | 🔴 |
| post-kube-scheduler-wasm-extension-push-images | kubernetes-sigs/kube-scheduler-wasm-extension | false | gcb-builder | k8s-staging-wasm-scheduler | 🔴 |
| post-windows-op-readiness-push-images | kubernetes-sigs/windows-operational-readiness | false | gcb-builder | k8s-staging-win-op-rdnss | 🔴 |
| post-windows-service-proxy-push-images | kubernetes-sigs/windows-service-proxy | false | gcb-builder | k8s-staging-win-svc-proxy | 🔴 |
| post-zeitgeist-push-images | kubernetes-sigs/zeitgeist | false | gcb-builder | k8s-staging-zeitgeist | 🔴 |
