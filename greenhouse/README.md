# Greenhouse

Greenhouse is our bazel [bazel remote caching](https://docs.bazel.build/versions/master/remote-caching.html) setup.
We use this to provide faster build & test presubmits with a Globally shared cache (per repo).

Most Bazel users should probably visit [the official docs](https://docs.bazel.build/versions/master/remote-caching.html) and select one of the options outlined there, with Prow/Kubernetes we are using a custom setup to explore:

- better support for multiple repos / cache invalidation by changing the cache URL suffix
  (see also: `images/bootstrap/create_bazel_cache_rcs.sh`)
- customized cache eviction logic
- integration with our logging and metrics stacks [WIP]

# Setup (on a Kubernetes Cluster)
We use this with [Prow](./../prow), to set it up we do the following:

 - Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) and [bazel](https://bazel.build/) and Point `KUBECONFIG` at your cluster.
   - for k8s.io use `make -C prow get-build-cluster-credentials`
 - Create a dedicated node. We use a GKE node-pool with a single node. Tag this node with label `dedicated=bazel-cache` and taint `dedicated=bazel-cache:NoSchedule` so your other tasks don't schedule on it.
   - for k8s.io this is:
   ```
   gcloud beta container node-pools create bazel-cache --cluster=prow --project=k8s-prow-builds --zone=us-central1-f --node-taints dedicated=bazel-cache:NoSchedule --machine-type=n1-standard-8 --num-nodes=1 --local-ssd-count=1
   kubectl label nodes $(kubectl get no | grep cache | cut -d" " -f1) dedicated=bazel-cache
   kubectl taint nodes $(kubectl get no | grep cache | cut -d" " -f1) dedicated=bazel-cache:NoSchedule
   ```
 - Create the Kubernetes service so jobs can talk to it conveniently: `kubectl apply -f greenhouse/service.yaml`
 - Finally build, push, and deploy with `bazel run //greenhouse:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64`
   <!--TODO(bentheelder): make this easier to consume by other users?-->
   - NOTE: other uses will likely need to tweak this step to their needs

