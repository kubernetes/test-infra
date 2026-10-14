# gcb-docker-gcloud image

This image has now been moved to the kubernetes/release [project](https://github.com/kubernetes/release/tree/master/images/releng/gcb-docker-gcloud) and is available at `registry.k8s.io/releng/gcb-docker-gcloud`. Its versioned by build date.

The image at registry.k8s.io has a breaking change that deletes the `/buildx-entrypoint.sh` script. You can migrate like this:

Old GCB Step

```yaml
  - name: gcr.io/k8s-staging-test-infra/gcb-docker-gcloud:v20260205-38cfa9523f
    entrypoint: /buildx-entrypoint
    args:
      - build
      - --tag=gcr.io/$PROJECT_ID/git-custom-k8s-auth:$_GIT_TAG
      - --push
      - .
    dir: .
```

New GCB Step

```yaml
  - name: registry.k8s.io/releng/gcb-docker-gcloud:v20260730
    args:
    - docker
    - buildx
    - build
    - --tag=gcr.io/$PROJECT_ID/kubekins-e2e:latest-$_CONFIG
    - --push
    - .
    dir: .
  # or call a make command, but don't configure buildx as we already handle it for you
  - name: registry.k8s.io/releng/gcb-docker-gcloud:v20260730
    args:
    - make
    - build
    dir: .
```
