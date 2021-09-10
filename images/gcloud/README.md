# gcloud-in-go

Use this image when you want to use `go` and `gcloud` in the same job

## contents

- base:
  - golang:1.13
- directories:
  - `/workspace` default working dir for `run` commands
- languages:
  - `python`
- tools:
  - `gcloud` installed via rapid channel, components include:
    - `alpha`
    - `beta`
    - `kubectl`
  - `rsync`
  - `wget`
