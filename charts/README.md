
# Kubernetes Test-Infra Helm charts

This is the [helm](https://github.com/kubernetes/helm) chart repository for test infra components. Helm is a package manager for kubernetes  this chart repository adapts test-infra components for general use on any kubernetes cluster. Please see documentation on individual components for more detail.

To use this repository to install prow:

```bash
helm repo add stable http://storage.googleapis.com/kubernetes-test-infra-charts
helm install stable/prow --set repos={"github_org/repo1" "github_org/repo2" ...}
```

