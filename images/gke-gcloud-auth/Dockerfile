FROM google/cloud-sdk:410.0.0-alpine

ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN gcloud components install gke-gcloud-auth-plugin
