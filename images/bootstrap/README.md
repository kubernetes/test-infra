# DEPRECATED

This image is deprecated and should not be used as the basis of any new images or used in prowjob configs.

Critical bugfies or security updates may be accepted, but approached with heavy skepticism.

New dependencies or features will very likely not be accepted.

# bootstrap image

This image is used as the base layer for the kubekins-e2e image, with a focus
on the deprecated bootstrap.py tooling that was used during Kubernetes' early
years.

It was built with assumptions in mind that are no longer or less relevant today
given the evolution of other test-infra, e.g. pod-utils, kubetest2

## contents

It comes with a laundry list of things:
- base:
  - debian as provided by `debian:buster`
- directories:
  - `/docker-graph` as docker's storage location
  - `/workspace` default working directory for `run` commands
    - `test-infra` a full clone of kubernetes/test-infra at build time
    - `scenarios` a copy of kubernetes/test-infra/scenarios at build time
- languages:
  - `python` with `pip`
  - `python3` with `pip`
- scripts:
  - `/usr/local/bin/entrypoint.sh` TODO
  - `/usr/local/bin/runner.sh` TODO
  - `/usr/local/bin/create_bazel_cache_rcs.sh` TODO
- tools:
  - `curl` and `wget`
  - `docker` via docker's apt repo
  - `gcloud` via rapid channel install, components include:
    - `alpha`
    - `beta`
    - `kubectl`
  - `git` and `hg`
  - `jq`
  - `rsync`
  - `zip`, `unzip`, and `xz-utils`
