# gcb-docker-gcloud image

This image is available for Google Cloud Build jobs that want to use a
combination of `docker`, `gcloud`, and `go` all in the same build step

## contents

- base:
  - golang:1.20.10-alpine
- languages:
  - `go`
- tools:
  - `docker`
  - `docker-buildx`, `qemu` binaries, `/buildx-entrypoint` for multi-arch support
  - `gcloud` via rapid channel, with default components
