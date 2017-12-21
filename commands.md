# k8s Bot Commands

`k8s-ci-robot` and `k8s-merge-robot` understand several commands. They should all be uttered on their own line, and they are case-sensitive.

For more detailed documentation on each of these commands, consult Prow's [plugin
help](https://prow.k8s.io/plugin-help.html).

Command | Implemented By | Who can run it | Description
--- | --- | --- | ---
`/approve` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | approve all the files for which you are an approver
`/approve no-issue` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | approve when a PR doesn't have an associated issue
`/approve cancel` | mungegithub [approvers](./mungegithub/mungers/approvers) | owners | removes your approval on this pull-request
`/area [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds an area/<> label(s) if it exists
`/remove-area [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes an area/<> label(s) if it exists
`/assign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Assigns specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/unassign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Unassigns specified people (or yourself if no one is specified). Target must already be assigned.
`/cc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Request review from specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/uncc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | anyone | Dismiss review request for specified people (or yourself if no one is specified). Target must already have had a review requested.
`/close` | prow [lifecycle](./prow/plugins/lifecycle) | authors and assignees | closes the issue/PR
`/reopen` | prow [lifecycle](./prow/plugins/lifecycle) | authors and assignees | reopens a closed issue/PR
`/lifecycle [state]` | prow [lifecycle](./prow/plugins/lifecycle) | anyone | adds a stale, rotten or frozen state label
`/remove-lifecycle [state]` | prow [lifecycle](./prow/plugins/lifecycle) | anyone | removes a stale, rotten or frozen state label
`/help` | prow [help](./prow/plugins/help) | anyone | adds the `help wanted` label
`/remove-help` | prow [help](./prow/plugins/help) | anyone | removes the `help wanted` label
`/hold` | prow [hold](./prow/plugins/hold) | anyone | adds the `do-not-merge/hold` label
`/hold cancel` | prow [hold](./prow/plugins/hold) | anyone | removes the `do-not-merge/hold` label
`/joke` | prow [yuks](./prow/plugins/yuks) | anyone | tells a bad joke, sometimes
`/kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a kind/<> label(s) if it exists
`/remove-kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a kind/<> label(s) if it exists
`/lgtm` | prow [lgtm](./prow/plugins/lgtm) | assignees | adds the `lgtm` label
`/lgtm cancel` | prow [lgtm](./prow/plugins/lgtm) | authors and assignees | removes the `lgtm` label
`/ok-to-test` | prow [trigger](./prow/plugins/trigger) | kubernetes org members | allows the PR author to `/test all`
`/test all`<br>`/test <some-test-name>` | prow [trigger](./prow/plugins/trigger) | anyone on trusted PRs | runs tests defined in [config.yaml](./prow/config.yaml)
`/retest` | prow [trigger](./prow/plugins/trigger) | anyone on trusted PRs | reruns failed tests
`/priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a priority/<> label(s) if it exists
`/remove-priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a priority/<> label(s) if it exists
`/sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | adds a sig/<> label(s) if it exists
`@kubernetes/sig-<some-github-team>` | prow [label](./prow/plugins/label) | kubernetes org members | adds the corresponding `sig` label
`/remove-sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | anyone | removes a sig/<> label(s) if it exists
`/release-note` | prow [releasenote](./prow/plugins/releasenote) | authors and kubernetes org members | adds the `release-note` label
`/release-note-action-required` | prow [releasenote](./prow/plugins/releasenote) | authors and kubernetes org members | adds the `release-note-action-required` label
`/release-note-none` | prow [releasenote](./prow/plugins/releasenote) | authors and kubernetes org members | adds the `release-note-none` label
`/status [label1 label2 ...]` | prow [milestonestatus](./prow/plugins/milestonestatus) | members of the [kubernetes-milestone-maintainers](https://github.com/orgs/kubernetes/teams/kubernetes-milestone-maintainers/members) github team | adds a status/<> label(s) if it exists
