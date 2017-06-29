
# Kubernetes Test-Infra Helm charts

This is the [helm](https://github.com/kubernetes/helm) chart repository for test infra components. Please see documentation on individual components for more detail.

To use this repository to install prow:

```bash
git clone https://github.com/kubernetes/test-infra.git
cd charts/prow
helm install . --set repos={"github_org/repo1" "github_org/repo2" ...}
```

