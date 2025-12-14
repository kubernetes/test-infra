# kubekins-e2e-v2

This is a modernised and lean image of the kubekins-e2e image optimised for the core tools required in an e2e image.

Features:
- multi-arch, supports amd64, arm64 , ppc64le and s390x
- kubetest2
- kind
- aws-cli v1
- gcloud
- DinD
- runner.sh wrapper

Its available at `us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:latest-master`.
If you are using it in a prowjob, please use a versioned tag(e.g. `v20251205-d1700a27d1-master` ) that will be autobumped.
