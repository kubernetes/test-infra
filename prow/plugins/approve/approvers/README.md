# Approvers plugin

The approvers plugin implements the [OWNERS file code review mechanism documented on the Kubernetes community site](https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md). See that site for a description of the expected behavior and features. The remainder of this document provides pointers to the internal implementation of the approvers (and LGTM) plugins.


## Configuration options

See the [Approve](https://godoc.org/k8s.io/test-infra/prow/plugins#Approve) go struct for documentation of the options for this plugin.

See also the [Lgtm](https://godoc.org/k8s.io/test-infra/prow/plugins#Lgtm) go struct for documentation of the [LGTM](#lgtm-label) plugin's options.

## Final Notes

Obtaining approvals from selected approvers is the last step towards merging a PR. The approvers approve a PR by typing `/approve` in a comment, or retract it by typing `/approve cancel`.

Algorithm for getting the status is as follow:

1. run through all comments to obtain latest intention of approvers

2. put all approvers into an approver set

3. determine whether a file has at least one approver in the approver set

4. add the status to the PR if all files have been approved

If an approval is cancelled, the bot will delete the status added to the PR and remove the approver from the approver set. If someone who is not an approver in the OWNERS file types `/approve` in a comment, the PR will not be approved. If someone who is an approver in the OWNERS file and s/he does not get selected, s/he can still type `/approve` or `/lgtm` in a comment, pushing the PR forward.

### Code Implementation Links

Blunderbuss: 
[prow/plugins/blunderbuss/blunderbuss.go](https://git.k8s.io/test-infra/prow/plugins/blunderbuss/blunderbuss.go)

LGTM:
[prow/plugins/lgtm/lgtm.go](https://git.k8s.io/test-infra/prow/plugins/lgtm/lgtm.go)

Approve:
[prow/plugins/approve/approve.go](https://git.k8s.io/test-infra/prow/plugins/approve/approve.go)

[prow/plugins/approve/approvers/owners.go](https://git.k8s.io/test-infra/prow/plugins/approve/approvers/owners.go)

