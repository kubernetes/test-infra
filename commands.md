# k8s Bot Commands

`k8s-ci-robot` and `k8s-merge-robot` understand several commands. They should all be uttered on their own line, and they are case-sensitive.

Command | Implemented By | Who can run it | Description
--- | --- | --- | ---
`/assign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Assigns specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/unassign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Unassigns specified people (or yourself if no one is specified). Target must already be assigned.
`/cc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Request review from specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/uncc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Dismiss review request for specified people (or yourself if no one is specified). Target must already have had a review requested.
`/area [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds an area/<> label(s) if it exists
`/remove-area [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes an area/<> label(s) if it exists
`/kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a kind/<> label(s) if it exists
`/remove-kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a kind/<> label(s) if it exists
`/priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a priority/<> label(s) if it exists
`/remove-priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a priority/<> label(s) if it exists
`/sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a sig/<> label(s) if it exists
`/remove-sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a sig/<> label(s) if it exists
`/lgtm` | prow [lgtm](./prow/plugins/lgtm) | assignees | adds the `lgtm` label
`/lgtm cancel` | prow [lgtm](./prow/plugins/lgtm) | authors and assignees | removes the `lgtm` label
`/hold` | prow [hold](./prow/plugins/hold) | anyone | adds the `hold` label
`/hold cancel` | prow [hold](./prow/plugins/hold) | anyone | removes the `hold` label
`/approve` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | approve all the files for which you are an approver
`/approve no-issue` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | approve when a PR doesn't have an associated issue
`/approve cancel` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | removes your approval on this pull-request
`/close` | prow [close](./prow/plugins/close) | authors and assignees | closes the issue/PR
`/reopen` | prow [reopen](./prow/plugins/reopen) | authors and assignees | reopens a closed issue/PR
`/release-note` | prow [releasenote](./prow/plugins/releasenote) | authors and kubernetes org members | adds the `release-note` label
`/release-note-none` | prow [releasenote](./prow/plugins/releasenote) | authors and kubernetes org members | adds the `release-note-none` label
`@kubernetes/sig-<some-github-team>` | prow [label](./prow/plugins/label) | kubernetes org members | adds the corresponding `sig` label
`/retest` | prow [trigger](./prow/plugins/trigger) | anyone on trusted PRs | reruns failed tests
`/test all`<br>`/test <some-test-name>` | prow [trigger](./prow/plugins/trigger) | anyone on trusted PRs | runs tests defined in [config.yaml](./prow/config.yaml)
`/ok-to-test` | prow [trigger](./prow/plugins/trigger) | kubernetes org members | allows the PR author to `/test all`
`/joke` | prow [yuks](./prow/plugins/yuks) | anyone | tells a bad joke, sometimes
