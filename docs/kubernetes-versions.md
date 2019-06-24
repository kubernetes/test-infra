# Kubernetes Versions

## CI Versions

Many periodic jobs related to `kubernetes/kubernetes` use an approximation of
channel based versions. At the time of writing, these map as follows.

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
$ for suffix in beta stable1 stable2 stable3; do 
  echo ci/k8s-$suffix: $(gsutil cat gs://kubernetes-release-dev/ci/k8s-$suffix.txt); 
done

ci/k8s-beta: v1.15.0-rc.1.19+e8462b5b5dc258
ci/k8s-stable1: v1.14.4-beta.0.3+d1c99590f4557a
ci/k8s-stable2: v1.13.8-beta.0.6+bda90cd6042eeb
ci/k8s-stable3: v1.12.10-beta.0.7+1f3029dd877ea8
```

## Release Versions

There are also file pointers for release builds, these correspond to file 
pointers in the `kubernetes-release` GCS bucket. For example, for the latest
release at the time of writing.

| channel     | tag                 |
| ----------- | ------------------- |
| latest-1.15 | v1.15.1-beta.0      |
| stable-1.15 | v1.15.0             |

```shell
$ for prefix in latest stable; do
  echo release/$prefix-1.11: $(gsutil cat gs://kubernetes-release/release/$prefix-1.11.txt)
  done

release/latest-1.11: v1.11.4-beta.0
release/stable-1.11: v1.11.3
```
