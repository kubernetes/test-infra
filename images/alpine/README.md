# alpine image

Use this image when you want to use `alpine` as base

## contents

- base:
  - `alpine:latest`

- build-arguments:
  - `IMAGE_ARG`:  
        This argument takes a tagged Docker image name.  
        For e.g. `IMAGE_ARG=gcr.io/$PROJECT_ID/alpine:$_GIT_TAG`
  - `AWS_IAM_AUTHENTICATOR_VERSION`:
        This argument specifies the version of `aws-iam-authenticator` plugin required for connecting to EKS clusters with AWS principals instead of Kubernetes service accounts.
- tools:
  - `ca-certificates`
  - `gke-gcloud-auth-plugin`
  - `aws-iam-authenticator`
