FROM google/cloud-sdk:410.0.0-alpine

ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apk add --no-cache git git-daemon openssh

COPY github-known-hosts /github_known_hosts
COPY ssh-config /etc/ssh/ssh_config

RUN gcloud components install gke-gcloud-auth-plugin