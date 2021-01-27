# Images

Each subdirectory corresponds to an image that is automatically built and pushed to gcr.io/k8s-testimages when PRs that touch them merge using [postsubmit prowjobs](https://testgrid.k8s.io/sig-testing-images) that run the [image-builder](/images/builder)

## Updating test-infra images

We have bazel images that have two versions of bazel installed. The upgrade process is as follows:
* Ensure [`.bazelversion`] file matches one of the versions in these three images:
  - [`images/bazel`]'s `test-infra` variant
  - [`images/bazelbuild`]'s `test-infra` variant
  - [`images/gcloud-bazel`]
* Choose the new target the [bazel release blog], such as `3.1`
  - Ensure the [`repo-infra` release] supports bazel >= the target vesion.
    - Look in [`load.bzl`] to see which repo-infra tag is used.
    - If necessary, update the `bazel_toolchains` version in [`repo-infra`'s `load.bzl`] to the latest [`bazel-toolchains` release].
    - Cut a new `repo-infra` release with this change.
    - Update `test-infra`'s `load.bzl` to use this `repo-infra` release.
* Create a PR to make images to include both the current and target version:
  - The old version should match what is currently in `.bazelversion`
  - The new version should be the target version.
  - [`images/bazel`]'s `test-infra` variant
  - [`images/bazelbuild`]'s `test-infra` variant
  - [`images/gcloud-bazel`] update the push.sh script
    - `TODO(fejta):` this should be done on postsubmit like the others
* Merge the PR
  - This should postsubmits to create the `bazel` and `bazelbuild` images.
  - Run the `push.sh` script in `images/gcloud-bazel` to push the new image.
* Update usage to these new images
  - Manually update the [`bazel-base`] digtest in `containers.bzl` to the new image.
    - NOTE: must update the digest (the tag param is just documentation)
    - `TODO(fejta):` this should be done automatically like the others
  - The periodic prow autobump job should make a PR to start using these images an hour later.
  - The next day oncall should merge this PR, at which point they will start getting used.
* Create a PR to change [`.bazelversion`] to the target version.
  - This should cause presubmits to try and use the new version.
  - Merging the PR will cause postsubmis/periodics to start using it.

## Testing Images

There is no automated testing pipeline for images:
- Any jobs that use the `:latest` tag use the latest published image immediately
- Any jobs that use a `:v{date}-{sha}[-{variant}]` tag (e.g. `:v20200407-c818676-master`) are updated to use the latest published image ~daily.  This is accomplished by PR's created by the [autobumper prowjob](https://testgrid.k8s.io/sig-testing-prow#autobump-prow), which are merged by [test-infra oncall](https://go.k8s.io/oncall) once a day during weekdays.

1. Merge a PR changing something in the image directory.

1. Grep the [prowjob configs](/config/jobs) to find out which jobs are using `gcr.io/k8s-testimages/<image-name>:latest` and monitor [TestGrid](http://testgrid.k8s.io) for new failures corresponding to your change.

    * On failure, send a new PR to rollback your last one or a fix if you know immediately.
    * Some of these images might be presubmits; you could monitor them at http://prow.k8s.io

1. You are done. If more breaks happen later, [test-infra oncall](go.k8s.io/oncall) will take care of it.


[`bazel-base`]: /containers.bzl
[`.bazelversion`]: /.bazelversion
[`images/bazel`]: /images/bazel/variants.yaml
[`images/bazelbuild`]: /images/bazelbuild/variants.yaml
[`images/gcloud-bazel`]: /images/gcloud-bazel/push.sh
[bazel release blog]: https://blog.bazel.build
[`repo-infra` release]: https://github.com/kubernetes/repo-infra/releases
[`load.bzl`]: /load.bzl
[`bazel_toolchains` release]: https://github.com/bazelbuild/bazel-toolchains/releases
[`repo-infra`'s `load.bzl`]: https://github.com/kubernetes/repo-infra/blob/master/load.bzl
