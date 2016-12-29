# Prow Commands

`k8s-ci-robot` understands several commands. They should all be uttered on their own line, and they are case-sensitive.

Command | Plugin | Who can run it | Description
--- | --- | --- | --- | ---
`/lgtm` | [lgtm](./plugins/lgtm) | assignees | adds the `lgtm` label
`/lgtm cancel` | [lgtm](./plugins/lgtm) | authors and assignees | removes the `lgtm` label
`/close` | [close](./plugins/close) | authors and assignees | closes the issue
`/release-note` | [releasenote](./plugins/releasenote) | authors and assignees | adds the `release-note` label
`/release-note-none` | [releasenote](./plugins/releasenote) | authors and assignees | adds the `release-note-none` label
`@k8s-bot test this` | [trigger](./plugins/trigger) | kubernetes org members | runs tests defined in [presubmit.yaml](./presubmit.yaml)
`@k8s-bot ok to test` | [trigger](./plugins/trigger) | kubernetes org members | allows the PR author to `@k8s-bot test this`
