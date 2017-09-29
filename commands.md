# k8s Bot Commands

`k8s-ci-robot` and `k8s-merge-robot` understand several commands. They should all be uttered on their own line, and they are case-sensitive.

Command | Implemented By | Who can run it | Description
--- | --- | --- | ---
`/approve` | mungegithub [approvers](./mungegithub/mungers/approvers) | Owners | Approve all the files for which you are an approver.
`/approve no-issue` | mungegithub [approvers](./mungegithub/mungers/approvers) | Owners | Approve when a PR doesn't have an associated issue.
`/approve cancel` | mungegithub [approvers](./mungegithub/mungers/approvers) | Owners | Removes your approval on this pull-request.
`/area [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Adds an area/<> label(s) if it exists.
`/remove-area [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Removes an area/<> label(s) if it exists.
`/assign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | Anyone | Assigns specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/unassign [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | Anyone | Unassigns specified people (or yourself if no one is specified). Target must already be assigned.
`/cc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | Anyone | Request review from specified people (or yourself if no one is specified). Target must be a kubernetes org member.
`/uncc [@userA @userB @etc]` | prow [assign](./prow/plugins/assign) | Anyone | Dismiss review request for specified people (or yourself if no one is specified). Target must already have had a review requested.
`/close` | prow [close](./prow/plugins/close) | Authors and assignees | Closes the issue/PR.
`/reopen` | prow [reopen](./prow/plugins/reopen) | Authors and assignees | Re-opens a closed issue/PR.
`/hold` | prow [hold](./prow/plugins/hold) | Anyone | Adds the `do-not-merge/hold` label.
`/hold cancel` | prow [hold](./prow/plugins/hold) | Anyone | Removes the `do-not-merge/hold` label.
`/joke` | prow [yuks](./prow/plugins/yuks) | Anyone | Tells a bad joke, sometimes.
`/kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Adds a kind/<> label(s) if it exists.
`/remove-kind [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Removes a kind/<> label(s) if it exists.
`/lgtm` | prow [lgtm](./prow/plugins/lgtm) | Assignees | Adds the `lgtm` label.
`/lgtm cancel` | prow [lgtm](./prow/plugins/lgtm) | Authors and assignees | Removes the `lgtm` label.
`/ok-to-test` | prow [trigger](./prow/plugins/trigger) | Kubernetes org members | Allows the PR author to `/test all`.
`/test all`<br>`/test <some-test-name>` | prow [trigger](./prow/plugins/trigger) | Anyone on trusted PRs | Runs tests defined in [config.yaml](./prow/config.yaml).
`/retest` | prow [trigger](./prow/plugins/trigger) | Anyone on trusted PRs | Re-runs failed tests.
`/priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Adds a priority/<> label(s) if it exists.
`/remove-priority [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Removes a priority/<> label(s) if it exists.
`/sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Adds a sig/<> label(s) if it exists.
`@kubernetes/sig-<some-github-team>` | prow [label](./prow/plugins/label) | Kubernetes org members | Adds the corresponding `sig` label.
`/remove-sig [label1 label2 ...]` | prow [label](./prow/plugins/label) | Anyone | Removes a sig/<> label(s) if it exists.
`/release-note` | prow [releasenote](./prow/plugins/releasenote) | Authors and kubernetes org members | Adds the `release-note` label.
`/release-note-action-required` | prow [releasenote](./prow/plugins/releasenote) | Authors and kubernetes org members | Adds the `release-note-action-required` label.
`/release-note-none` | prow [releasenote](./prow/plugins/releasenote) | Authors and kubernetes org members | Adds the `release-note-none` label.
