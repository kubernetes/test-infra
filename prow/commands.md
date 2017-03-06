# Prow Commands

`k8s-ci-robot` understands several commands. They should all be uttered on their own line, and they are case-sensitive.

Command | Plugin | Who can run it | Description
--- | --- | --- | --- | ---
`/assign [@userA @userB @etc]` | [assign](./plugins/assign) | org members | Assigns specified people (or yourself if no one is specified)
`/unassign [@userA @userB @etc]` | [assign](./plugins/assign) | org members | Unassigns specified people (or yourself if no one is specified)
`/lgtm` | [lgtm](./plugins/lgtm) | assignees | adds the `lgtm` label
`/lgtm cancel` | [lgtm](./plugins/lgtm) | authors and assignees | removes the `lgtm` label
`/close` | [close](./plugins/close) | authors and assignees | closes the issue
`/release-note` | [releasenote](./plugins/releasenote) | authors and assignees | adds the `release-note` label
`/release-note-none` | [releasenote](./plugins/releasenote) | authors and assignees | adds the `release-note-none` label
`@k8s-bot test this` | [trigger](./plugins/trigger) | kubernetes org members | runs tests defined in [config.yaml](./config.yaml)
`@k8s-bot ok to test` | [trigger](./plugins/trigger) | kubernetes org members | allows the PR author to `@k8s-bot test this`
`@k8s-bot tell me a joke` | [yuks](./plugins/yuks) | anyone | tells a bad joke, sometimes
