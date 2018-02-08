# Nursery

Nursery is our bazel [bazel remote caching](https://docs.bazel.build/versions/master/remote-caching.html) setup.

Initially we are using https://github.com/buchgr/bazel-remote but if this goes
well we will probably want to implement something mores sophisticated.

Setup (for [prow.k8s.io](https://prow.k8s.io/)):
```
make -C prow get-build-cluster-credentials
gcloud beta container node-pools create bazel-cache --cluster=prow --project=k8s-prow-builds --zone=us-central1-f --node-taints dedicated=bazel-cache:NoSchedule --machine-type=n1-standard-8 --num-nodes=1 --local-ssd-count=1
kubectl label nodes $(kubectl get no | grep cache | cut -d" " -f1) dedicated=bazel-cache
kubectl taint nodes $(kubectl get no | grep cache | cut -d" " -f1) dedicated=bazel-cache:NoSchedule
kubectl apply -f experiment/nursery/deployment.yaml
kubectl apply -f experiment/nursery/service.yaml
```
