# Contributing to kubernetes/test-infra

Make a [pull request](https://help.github.com/articles/about-pull-requests/) (hereafter PR). 
In the PR body, feel free to add an area label if appropriate by saying `/area <AREA>`. 
The list of labels is [here](https://github.com/kubernetes/test-infra/labels). 
Also feel free to suggest a reviewer with `/assign @theirname`.

Once your reviewer is happy, they will say `/lgtm` which will apply the 
`lgtm` label, and will apply the `approved` label if they are an 
[owner](/OWNERS).
The `approved` label will also automatically be applied to PRs opened by an 
OWNER. If neither you nor your reviewer is an owner, please `/assign` someone
 who is.
Your PR will be automatically merged once it has the the `lgtm` and `approved` 
labels, does not have any `do-not-merge/*` labels, and all tests are passing.
