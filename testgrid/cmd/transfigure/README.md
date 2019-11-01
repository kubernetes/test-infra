# Transfigure.sh

Transfigure is an image that generates a YAML TestGrid configuration from a Prow configuration and pushes it to be used on [testgrid.k8s.io].
It is used specifically for Prow instances other than the k8s instance of Prow.

## Requirements
`transfigure.sh` takes, as an argument, a GitHub personal access token that has been granted the `repo` access scope.
The following can be provided by further arguments or by environment variables:

* `CONFIG_PATH`: Prow's\* Config path
* `JOB_CONFIG`: Prow's\* Job Config path
* `TESTGRID_CONFIG`: TestGrid configuration directory or default file
* `ORG_REPO`: The subdirectory in `/config/testgrids/...` to push to. Usually `<github_org>/<github_repository>`.

\*"Prow" refers to your non-k8s Prow instance

## Building
The `gcr.io/k8s-prow/transfigure` image is built and published automatically by [`post-test-infra-push-prow`](https://github.com/kubernetes/test-infra/blob/9a939de10fa72af415eb1e628345b7d16c1f0be0/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L118-L143) with the rest of the Prow components.

You can build the image locally and use it in Docker with `bazel run //testgrid/cmd/transfigure`. Publish to a remote repository after building with `docker push` or build and push all Prow images at once with [`prow/push.sh`](/prow/push.sh).

[testgrid.k8s.io]: (https://testgrid.k8s.io/)
