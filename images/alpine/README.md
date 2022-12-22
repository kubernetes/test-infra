# alpine image

Use this image when you want to use `alpine` as base

## contents

- base:
  - `alpine:latest`

- build-arguments:
  - `IMAGE_ARG`:  
        This argument takes a tagged Docker image name.  
        For e.g. `IMAGE_ARG=gcr.io/$PROJECT_ID/alpine:$_GIT_TAG`

- tools:
  - `ca-certificates`
