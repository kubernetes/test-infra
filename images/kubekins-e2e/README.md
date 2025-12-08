# Deprecated

This image is deprecated in favour of kubekins-e2e-v2 image. Please use that image if you don't need the legacy CI kruft such as:
- bootstrap
- kubetest1
- scenarios/*
- various unmaintained tools such as logexporter, etc

Its available at `gcr.io/k8s-staging-test-infra/kubekins-e2e:latest-master`. If you are using it in a prowjob, please use a versioned tag(eg: `v20251205-d1700a27d1-master` ) that will be autobumped.
