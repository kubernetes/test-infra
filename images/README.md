# Images

Each subdirectory corresponds to an image that is automatically built and pushed to gcr.io/k8s-testimages when PRs that touch them merge using [postsubmit prowjobs](https://testgrid.k8s.io/sig-testing-images) that run the [image-builder](/images/builder)

## Testing Images

There is no automated testing pipeline for images:
- Any jobs that use the `:latest` tag use the latest published image immediately
- Any jobs that use a `:v{date}-{sha}[-{variant}]` tag (e.g. `:v20200407-c818676-master`) are updated to use the latest published image ~daily.  This is accomplished by PR's created by the [autobumper prowjob](https://testgrid.k8s.io/sig-testing-prow#autobump-prow), which are merged by [test-infra oncall](go.k8s.io/oncall) once a day during weekdays.

1. Merge a PR changing something in the image directory.

1. Grep the [prowjob configs](/config/jobs) to find out which jobs are using `gcr.io/k8s-testimages/<image-name>:latest` and monitor [TestGrid](http://testgrid.k8s.io) for new failures corresponding to your change.

    * On failure, send a new PR to rollback your last one or a fix if you know immediately.
    * Some of these images might be presubmits; you could monitor them at http://prow.k8s.io

1. You are done. If more breaks happen later, [test-infra oncall](go.k8s.io/oncall) will take care of it.
