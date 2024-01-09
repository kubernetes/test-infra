# gcloud-terraform image

Use this image when you want to use `gcloud` and `terraform` in the same job

## contents

- base:
  - `gcr.io/k8s-prow/alpine:v20240108-a28886d2bd`
- directories:
  - `/workspace` default working dir for `run` commands
- languages:
  - `python3`
- tools:
  - `bash`
  - `curl` 
  - `gcloud` installed via rapid channel, components include:
    - `alpha`
    - `beta`
    - `kubectl`
  - `make`
  - `rsync`
  - `terraform` v0.14.9-r1
  - `wget`
