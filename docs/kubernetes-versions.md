# Kubernetes Versions

## CI Versions

Many periodic jobs related to `kubernetes/kubernetes` use an approximation of
channel based versions.

At the time of writing (post-`release-1.16` branch creation), these map as follows:

| channel | branch       |
| ------- | ------------ |
| dev     | master       |
| beta    | release-1.15 |
| stable1 | release-1.14 |
| stable2 | release-1.13 |
| stable3 | release-1.12 |

The names correspond to file pointers in the `kubernetes-release-dev` GCS
bucket that are updated by CI

```shell
for suffix in beta stable1 stable2 stable3; do
  echo ci/k8s-$suffix: $(gsutil cat gs://kubernetes-release-dev/ci/k8s-$suffix.txt);
done

ci/k8s-beta: v1.15.3-beta.0.39+3f2d0c735c4a92
ci/k8s-stable1: v1.14.6-beta.0.33+f0c11ee08cd6b2
ci/k8s-stable2: v1.13.10-beta.0.16+48844ef5e7cf96
ci/k8s-stable3: v1.12.11-beta.0.1+5f799a487b70ae
```

## Release Versions

There are also file pointers for release builds, these correspond to file 
pointers in the `kubernetes-release` GCS bucket.

For example, for the latest release at the time of writing (post-`release-1.16` branch creation):

| channel     | tag                 |
| ----------- | ------------------- |
| latest-1.15 | v1.15.3-beta.0      |
| stable-1.15 | v1.15.2             |

```shell
for prefix in latest stable; do
  echo release/$prefix-1.16: $(gsutil cat gs://kubernetes-release/release/$prefix-1.16.txt)
done

release/latest-1.15: v1.15.3-beta.0
release/stable-1.15: v1.15.2
```
