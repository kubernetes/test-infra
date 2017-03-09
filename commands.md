# k8s Bot Commands

`k8s-ci-robot` and `k8s-merge-robot` understand several commands. They should all be uttered on their own line, and they are case-sensitive.

Command | Implemented By | Who can run it | Description
--- | --- | --- | ---
`/assign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | kubernetes org members | Assigns specified people (or yourself if no one is specified)
`/unassign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | kubernetes org members | Unassigns specified people (or yourself if no one is specified)
`/area` | prow [label](./prow/plugins/label) | anyone | adds an area/<> label if it exists
`/kind` | prow [label](./prow/plugins/label) | anyone | adds a kind/<> label if it exists
`/priority` | prow [label](./prow/plugins/label) | anyone | adds a priority/<> label if it exists
`/lgtm` | prow [lgtm](./prow/plugins/lgtm) | assignees | adds the `lgtm` label
`/lgtm cancel` | prow [lgtm](./prow/plugins/lgtm) | authors and assignees | removes the `lgtm` label
`/approve` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | approve all the files for which you are an approver
`/approve cancel` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | removes your approval on this pull-request
`/close` | prow [close](./prow/plugins/close) | authors and assignees | closes the issue
`/release-note` | prow [releasenote](./prow/plugins/releasenote) | authors and assignees | adds the `release-note` label
`/release-note-none` | prow [releasenote](./prow/plugins/releasenote) | authors and assignees | adds the `release-note-none` label
`/sig-<some-github-team>` | prow [label](./prow/plugins/label) | kubernetes org members| adds the corresponding `sig` label
`@k8s-bot test this` | prow [trigger](./prow/plugins/trigger) | kubernetes org members | runs tests defined in [config.yaml](./config.yaml)
`@k8s-bot ok to test` | prow [trigger](./prow/plugins/trigger) | kubernetes org members | allows the PR author to `@k8s-bot test this`
`@k8s-bot tell me a joke` | prow [yuks](./prow/plugins/yuks) | anyone | tells a bad joke, sometimes
