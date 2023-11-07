# git image

Use this image when you want to use `git` and `alpine` as base in a job.

## contents

- base:
  - `gcr.io/k8s-prow/alpine:v20231107-7fb7c64d33`
- directories:
  - `/github_known_hosts` holds the [`github-known-hosts`](/images/git/github-known-hosts) file copied during image build
  - `/etc/ssh/ssh_config` contains [`ssh-config`](/images/git/ssh-config) file copied during image build
- tools:
  - `git`
  - `openssh`
