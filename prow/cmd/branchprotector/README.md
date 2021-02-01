# Branch Protection Documentation

branchprotector configures [github branch protection] according to a specified
policy.

## Policy configuration

Extend the primary prow [`config.yaml`] document to include a top-level
`branch-protection` key that looks like the following:

```yaml

branch-protection:
  orgs:
    kubernetes:
      repos:
        test-infra:
          # Protect all branches in kubernetes/test-infra
          protect: true
          # Always allow the org's oncall-team to push
          restrictions:
            teams: ["oncall-team"]
          # Ensure that the extra-process-followed github status context passes.
          # In addition, adds any required prow jobs (aka always_run: true)
          required_status_checks:
            contexts: ["extra-process-followed"]

presubmits:
  kubernetes/test-infra:
  - name: fancy-job-name
    context: fancy-job-name
    always_run: true
    spec:  # podspec that runs job
```

This config will:
  * Enable protection for every branch in the `kubernetes/test-infra`
repo.
  * Require `extra-process-followed` and `fancy-job-name` [status contexts] to pass
    before allowing a merge
    - Although it will always allow `oncall-team` to merge, even if required
      contexts fail.
    - Note that `fancy-job-name` is pulled in automatically from the
      `presubmits` config for the repo, if one exists.

### Updating

* Send PR with `config.yaml` changes
* Merge PR
* Done!

Make changes to the policy by modifying [`config.yaml`] in your favorite text
editor and then send out a PR. When the PR merges prow pushes the updated config
. The branchprotector applies the new policies the next time it runs (within
24hrs).

### Advanced configuration


#### Fields

See [`branch_protection.go`] and GitHub's [protection api] for a complete list of fields allowed
inside `branch-protection` and their meanings. The format is:

```yaml
branch-protection:
  # default policy here
  orgs:
    foo:
      # this is the foo org policy
      protect: true  # enable protection
      enforce_admins: true  # rules apply to admins
      required_linear_history: true  # enforces a linear commit Git history
      allow_force_pushes: true  # permits force pushes to the protected branch
      allow_deletions: true  # allows deletion of the protected branch
      required_pull_request_reviews:
        dismiss_stale_reviews: false # automatically dismiss old reviews
        dismissal_restrictions: # allow review dismissals
          users:
          - her
          - him
          teams:
          - them
          - those
        require_code_owner_reviews: true  # require a code owner approval
        required_approving_review_count: 1 # number of approvals
      required_status_checks:
        strict: false # require pr branch to be up to date
        contexts: # checks which must be green to merge
        - foo
        - bar
      restrictions: # restrict who can push to the repo
        users:
        - her
        - him
        teams:
        - them
        - those
```



#### Scope


It is possible to define a policy at the
`branch-protection`, `org`, `repo` or `branch` level. For example:

```yaml
branch-protection:
  # Protect unless overridden
  protect: true
  # If protected, always require the cla status context
  required_status_checks:
    contexts: ["cla"]
  orgs:
    unprotected-org:
      # Disable protection unless overridden (overrides parent setting of true)
      protect: false
      repos:
        protected-repo:
          protect: true
          # Inherit protect-by-default config from parent
          # If protected, always require the tested status context
          required_status_checks:
            contexts: ["tested"]
          branches:
            secure:
              # Protect the secure branch (overrides inhereted parent setting of false)
              protect: true
              # Require the foo status context
              required_status_checks:
                contexts: ["foo"]
    different-org:
      # Inherits protect-by-default: true setting from above
```

The general rule for how to compute child values is:
  * If the child value is `null` or missing, inherit the parent value.
  * Otherwise:
    -   List values (like `contexts`), create a union of the parent and child lists.
    -   For bool/int values (like `protect`), the child value replaces the parent value.

So in the example above:
  * The `secure` branch in `unprotected-org/protected-repo`
    - enables protection (set a branch level)
    - requires `foo` `tested` `cla` [status contexts]
      (the latter two are appended by ancestors)
  * All other branches in `unprotected-org/protected-repo`
    - disable protection (inherited from org level)
  * All branches in all other repos in `unprotected-org`
    - disable protection (set at org level)
  * All branches in all repos in `different-org`
    - Enable protection (inherited from branch-protection level)
    - Require the `cla` context to be green to merge (appended by parent)

## Developer docs

Use [`planter.sh`] if [`bazel`] is not already installed on the machine.

### Run unit tests

`bazel test //prow/cmd/branchprotector:all`

### Run locally

`bazel run //prow/cmd/branchprotector -- --help`, which will tell you about the
current flags.

Do a dry run (which will not make any changes to github) with
something like the following command:

```sh
bazel run //prow/cmd/branchprotector -- \
  --config-path=/path/to/config.yaml \
  --github-token-path=/path/to/my-github-token
```

This will say how the binary will actually change github if you add a
`--confirm` flag.

### Deploy local changes to dev cluster

Run things like the following:
```sh
# Build and push image, create job
bazel run //prow/cmd/branchprotector:oneshot.create
# Delete finished job
bazel run //prow/cmd/branchprotector:oneshot.delete
# Describe current state of job
bazel run //prow/cmd/branchprotector:oneshot.describe
```

This will build an image with your local changes, push it to
`STABLE_DOCKER_REPO` and then deploy to `STABLE_PROW_CLUSTER`.

See [`print-workspace-status.sh`] for the definition of these variables.
See [`oneshot-job.yaml`] for details on the deployed job resource.

### Deploy cronjob to production

Follow the standard prow deployment process:

```sh
# build and push image, update tag used in production
prow/bump.sh branchprotector
# apply changes to production
bazel run //config/prow/cluster:branchprotector.apply
```

See [`prow/bump.sh`] for details on this script.
See [`config/prow/cluster/branchprotector_cronjob.yaml`] for details on the deployed
job resource.


[`bazel`]: https://bazel.build
[`branch_protection.go`]: /prow/config/branch_protection.go
[`config.yaml`]: /config/prow/config.yaml
[github branch protection]: https://help.github.com/articles/about-protected-branches/
[`oneshot-job.yaml`]: oneshot-job.yaml
[`planter.sh`]: /planter
[`print-workspace-status.sh`]: ../../../hack/print-workspace-status.sh
[`prow/bump.sh`]: /prow/bump.sh
[`config/prow/cluster/branchprotector_cronjob.yaml`]: /config/prow/cluster/branchprotector_cronjob.yaml
[status contexts]: https://developer.github.com/v3/repos/statuses/#create-a-status
[protection api]: https://developer.github.com/v3/repos/branches/#update-branch-protection
