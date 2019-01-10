# `status-reconciler`

`status-reconciler` ensures that changes to blocking presubmits in Prow configuration while PRs are
in flight do not cause those PRs to get stuck.

When the set of blocking presubmits changes for a repository, one of three cases occurs:

 - a new blocking presubmit exists and should be triggered for every trusted pull request in flight
 - an existing blocking presubmit is removed and should have its' status retired
 - an existing blocking presubmit is renamed and should have its' status migrated

The `status-reconciler` watches the job configuration for Prow and ensures that the above actions
are taken as necessary.

Note that `status-reconciler` is edge driven (not level driven) so it can't be used retrospectively.
To update statuses that were stale before deploying `status-reconciler`,
you can use the [`migratestatus`](/maintenance/migratestatus) tool.
