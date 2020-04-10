# Images README

## Testing Images

Images are tested in production.

1. Push a PR changing something in the image directory, preferably not near the time the Prow auto-bump occurs (otherwise, it will hard to figure out the true failure reason).

1. Grep the [prow configs](../config/jobs) to find out which jobs are using <image-name>:latest and monitor [the TestGrid](http://testgrid.k8s.io) for new failures corresponding to your change.

    * On failure, send a new PR to rollback your last one or a fix if you know immediately.
    * Some of these images might be presubmits; you could monitor them at http://prow.k8s.io

1. You are done. If more breaks happen later, the oncall will take care of it.

