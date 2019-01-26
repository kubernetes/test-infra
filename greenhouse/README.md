# Greenhouse

Greenhouse is our [bazel remote caching](https://docs.bazel.build/versions/master/remote-caching.html) setup.
We use this to provide faster build & test presubmits with a Globally shared cache (per repo).

We have a dashboard with metrics at: [velodrome.k8s.io/dashboard/db/bazel-cache](http://velodrome.k8s.io/dashboard/db/bazel-cache?orgId=1)

Most Bazel users should probably visit [the official docs](https://docs.bazel.build/versions/master/remote-caching.html) and select one of the options outlined there, with Prow/Kubernetes we are using a custom setup to explore:

- better support for multiple repos / cache invalidation by changing the cache URL suffix
  (see also: `images/bootstrap/create_bazel_cache_rcs.sh`)
- customized cache eviction / management
- integration with our logging and metrics stacks


## Setup (on a Kubernetes Cluster)
We use this with [Prow](./../prow), to set it up we do the following:

 - Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) and [bazel](https://bazel.build/) and Point `KUBECONFIG` at your cluster.
   - for k8s.io use `make -C prow get-build-cluster-credentials`
 - Create a dedicated node. We use a GKE node-pool with a single node. Tag this node with label `dedicated=greenhouse` and taint `dedicated=greenhouse:NoSchedule` so your other tasks don't schedule on it.
   - for k8s.io (running on GKE) this is:
   ```
   gcloud beta container node-pools create greenhouse --cluster=prow --project=k8s-prow-builds --zone=us-central1-f --node-taints=dedicated=greenhouse:NoSchedule --node-labels=dedicated=greenhouse --machine-type=n1-standard-32 --num-nodes=1
   ```
   - if you're not on GKE you'll probably want to pick a node to dedicate and do something like:
   ```
   kubectl label nodes $GREENHOUSE_NODE_NAME dedicated=greenhouse
   kubectl taint nodes $GREENHOUSE_NODE_NAME dedicated=greenhouse:NoSchedule
   ```
 - Create the Kubernetes service so jobs can talk to it conveniently: `kubectl apply -f greenhouse/service.yaml`
 - Create a `StorageClass` / `PersistentVolumeClaim` for fast cache storage, we use `kubectl apply -f greenhouse/gce-fast-storage.yaml` for 3TB of pd-ssd storage
 - Finally build, push, and deploy with `bazel run //greenhouse:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64`
   <!--TODO(bentheelder): make this easier to consume by other users?-->
   - NOTE: other uses will likely need to tweak this step to their needs, particular the service and storage definitions


## Optional Setup:
- tweak `metrics-service.yaml` and point prometheus at this service to collect metrics
