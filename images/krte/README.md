# krte image

krte - [KIND](https://sigs.k8s.io/kind) RunTime Environment

This image contains things we need to run kind in Kubernetes CI, and
is maintained for the sole purpose of testing Kubernetes with KIND.

## WARNING

This image is _not_ supported for other use cases. Use at your own risk.

## Build-time variables
See the `ARG` instructions in [`Dockerfile`](./Dockerfile).

## Run-time variables
- `KRTE_SYSTEMD=true` (default: `false`): enable systemd
- `KRTE_SYSTEMD_ROOTLESS=true` (default: `false`): switch to a non-root user via systemd.
  The KRTE container itself still has to be run as the root, so DO NOT specify `securityContext.runAsUser`.
