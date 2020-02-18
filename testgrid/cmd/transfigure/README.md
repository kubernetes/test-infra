# Transfigure.sh

Transfigure is an image that generates a YAML TestGrid configuration from a Prow configuration and pushes it to be used on [testgrid.k8s.io].
It is used specifically for Prow instances other than the k8s instance of Prow.

## Usage with Prow

To have your instance of Prow push to TestGrid automatically, add a [Prow Job](/prow/jobs.md)
like the ones in this [example file](./example-prowjobs.yaml).

## Arguments

`transfigure.sh` takes several arguments.

* `[github_token]` : A GitHub personal access token that has been granted the `repo` access scope.
  This is usually a robot token mounted as a volume, as the example above illustrates.
  Ensure that this user has [signed the CLA](https://github.com/kubernetes/community/blob/master/CLA.md#the-contributor-license-agreement)
  to contribute to this repository.
* `[prow_config]`: Prow's\* Config path
* `[prow_job_config]`: Prow's\* Job Config path
* `[testgrid_yaml]`: TestGrid configuration directory or default file
* `[repo_subdir]`: The subdirectory in [`/config/testgrids/...`](/config/testgrids) to push to.
  Usually `<github_org>` or `<github_org>/<github_repository>`.
* `(remote_fork_repo)`: Your user needs to have a fork of
  [`kubernetes/test-infra`](/). If it does, but that fork _isn't_ named `test-infra`,
  specify the name here. This is usually because your user already has a
  different repo as `test-infra` when `kubernetes/test-infra` was forked.

\*"Prow" refers to your non-k8s Prow instance

## Building

The `gcr.io/k8s-prow/transfigure` image is built and published automatically by [`post-test-infra-push-prow`](https://github.com/kubernetes/test-infra/blob/9a939de10fa72af415eb1e628345b7d16c1f0be0/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L118-L143) with the rest of the Prow components.

You can build the image locally and use it in Docker with `bazel run //testgrid/cmd/transfigure`. Publish to a remote repository after building with `docker push` or build and push all Prow images at once with [`prow/push.sh`](/prow/push.sh).

[testgrid.k8s.io]: (https://testgrid.k8s.io/)
