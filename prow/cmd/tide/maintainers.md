# Maintainer's Guide to Tide

## Best practices

1. Don't let humans (or other bots) merge especially if tests have a long duration. Every merge invalidates currently running tests for that pool.
1. Try to limit the total number of queries that you configure. Individual queries can cover many repos and include many criteria without using additional API tokens, but separate queries each require additional API tokens.
1. Ensure that merge requirements configured in GitHub match the merge requirements configured for Tide. If the requirements differ, Tide may try to merge a PR that GitHub considers unmergeable.
1. If you are using the `lgtm` plugin and requiring the `lgtm` label for merge, don't make queries exclude the `needs-ok-to-test` label. The `lgtm` plugin triggers one round of testing when applied to an untrusted PR and removes the `lgtm` label if the PR changes so it indicates to Tide that the current version of the PR is considered trusted and can be retested safely.

## Expected behavior that might seem strange

1. Any merge to a pool kicks all other PRs in the pool back into `Queued for retest`. This is because Tide requires PRs to be tested against the most recent base branch commit in order to be merged. When a merge occurs, the base branch updates so any existing or in-progress tests can no longer be used to qualify PRs for merge. All remaining PRs in the pool must be retested.
1. Waiting to merge a successful PR because a batch is pending. This is because Tide prioritizes batches over individual PRs and the previous point tells us that merging the individual PR would invalidate the pending batch. In this case Tide will wait for the batch to complete and will merge the individual PR only if the batch fails. If the batch succeeds, the batch is merged.
1. If the merge requirements for a pool change it may be necessary to "poke" or "bump" PRs to trigger an update on the PRs so that Tide will resync the status context. Alternatively, Tide can be restarted to resync all statuses.
1. Tide may merge a PR without retesting if the existing test results are already against the latest base branch commit.
1. It is possible for `tide` status contexts on PRs to temporarily differ from the Tide dashboard or Tide's behavior. This is because status contexts are updated asynchronously from the main Tide sync loop and have a separate rate limit and loop period.

## Other resources

- [Configuring Tide](/prow/cmd/tide/config.md)