# Kubernetes Versions

## CI Versions

Many periodic jobs related to `kubernetes/kubernetes` use an approximation of
channel based versions.

At the time of writing (pre-`release-1.19` branch creation), these map as follows:

| channel    | branch       |
| ---------- | ------------ |
| ci/master  | master       |
| ci/beta    | release-1.18 |
| ci/stable1 | release-1.17 |
| ci/stable2 | release-1.16 |
| ci/stable3 | release-1.15 |

The names correspond to file pointers in the `kubernetes-release-dev` GCS
bucket that are updated by CI

```shell
for suffix in master beta stable1 stable2 stable3; do
  echo ci/k8s-$suffix: $(gsutil cat gs://kubernetes-release-dev/ci/k8s-$suffix.txt);
done

ci/k8s-master: v1.19.0-beta.2.607+4c853bb28f57f8
ci/k8s-beta: v1.18.6-rc.0.15+e38139724f8f00
ci/k8s-stable1: v1.17.9-rc.0.13+fdec14cd4c84b7
ci/k8s-stable2: v1.16.13-rc.0.5+24212641c73bc0
ci/k8s-stable3: v1.15.13-beta.0.1+a34f1e483104bd
```

## Release Versions

There are also file pointers for release builds, these correspond to file 
pointers in the `kubernetes-release` GCS bucket.

For example, for the latest release at the time of writing (pre-`release-1.19` branch creation):

| channel             | tag                 |
| ------------------- | ------------------- |
| release/latest-1.15 | v1.15.13-beta.0 |
| release/stable-1.15 | v1.15.12 |
| release/latest-1.16 | v1.16.13-rc.0 |
| release/stable-1.16 | v1.16.12 |
| release/latest-1.17 | v1.17.9-rc.0 |
| release/stable-1.17 | v1.17.8 |
| release/latest-1.18 | v1.18.6-rc.0 |
| release/stable-1.18 | v1.18.5 |
| release/latest-1.19 | v1.19.0-beta.2 |
| release/stable-1.19 | n/a |

```shell
for n in $(seq 15 19); do
  for prefix in latest stable; do
    echo release/$prefix-1.$n: $(gsutil cat gs://kubernetes-release/release/$prefix-1.$n.txt)
  done
done

release/latest-1.15: v1.15.13-beta.0
release/stable-1.15: v1.15.12
release/latest-1.16: v1.16.13-rc.0
release/stable-1.16: v1.16.12
release/latest-1.17: v1.17.9-rc.0
release/stable-1.17: v1.17.8
release/latest-1.18: v1.18.6-rc.0
release/stable-1.18: v1.18.5
release/latest-1.19: v1.19.0-beta.2
CommandException: No URLs matched: gs://kubernetes-release/release/stable-1.19.txt
release/stable-1.19:
```
