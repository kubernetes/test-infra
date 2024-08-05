# kubekins-e2e-v2

This is a modernised and lean image of the kubekins-e2e image optimised for the core tools required in an e2e image.

This image is available at `us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:v20240705-131cd74733-master`

Features:
- multi-arch, supports both amd64 and arm64
- kubetest2
- kind
- aws-cli v2
- gcloud
- DinD
- runner.sh wrapper
