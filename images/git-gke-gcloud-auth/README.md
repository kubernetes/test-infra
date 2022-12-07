# git-gke-gcloud-auth image

Use this image when you want to use `git`, `gke-gcloud-auth-plugin` and `alpine` as base in a job.

## contents

- base:
  - `google/cloud-sdk:410.0.0-alpine`
- directories:
  - `/github_known_hosts` holds the [`github-known-hosts`](/images/git/github-known-hosts) file copied during image build
  - `/etc/ssh/ssh_config` contains [`ssh-config`](/images/git/ssh-config) file copied during image build
- tools:
  - `gcloud`
  - `git`
  - `openssh`
