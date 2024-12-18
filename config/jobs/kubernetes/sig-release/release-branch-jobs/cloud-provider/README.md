# How to generate the k8sbeta job in this folder

When a release branch of kubernetes is first cut, the jobs defined in [`cloud_provider_image_validation.yaml`]
must be forked to use the new release branch. Use [`releng/config-forker`] to
accomplish this, eg:

```sh
# from test-infra root
$ go run ./releng/config-forker \
  --job-config $(pwd)/releng/cloud_provider_image_validation.yaml \
  --version 1.27 \
  --go-version 1.31 \
  --output $(pwd)/config/jobs/kubernetes/sig-release/release-branch-jobs/cloud-provider/image-validation-1.31.yaml
```

# How to rotate the k8sbeta job to stable1

```sh
# from test-infra root
$ go run ./releng/config-rotator \
  --config-file ./config/jobs/kubernetes/sig-release/release-branch-jobs/cloud-provider/image-validation-1.31.yaml \
  --new stable1 --old beta
```


[`releng/config-forker`]: /releng/config-forker
[`cloud_provider_image_validation.yaml`]: /releng/cloud_provider_image_validation.yaml
