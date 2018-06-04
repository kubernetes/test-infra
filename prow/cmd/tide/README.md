# Tide

Tide is a [Prow](https://github.com/kubernetes/test-infra/blob/master/prow/README.md)
component for managing a pool of PRs that match a given set of criteria.
It will automatically retest and merge PRs in the pool if they pass tests
against the latest base branch commit.

Configuration of Tide is located under
[prow/config.yaml](https://github.com/kubernetes/test-infra/blob/master/prow/config.yaml)
file. It consists of two parts:

  * **Specification of PR pools/criteria**:
    A set of all PRs Tide keeps track of.
    The PRs are periodically checked if they are ready to be merged.
    Configured under `tide` key of the configuration file.

  * **Specification of PR tests**:
    A set of jobs that are run over each PR before it gets merged.
    No job is required to be provided in which case it's enough if a PR has all
    relevant labels set. If a set of jobs is specified,
    all must succeed (unless a job is configured with `skip_report: true`).
    Configured under `presubmits` key of the configuration file.

## PR pools

The set of criteria is specified through the following collection of items:

```yaml
tide:
  queries:                // List of queries
  - repo:                   // org/repo
    labels:                 // List of must have labels
    missingLabels:          // List of can't have labels
    excludedBranches:       // Ignore branches
    includedBranches:       // Include-only branches
    reviewApprovedRequired: // Review approved is must
  sync_period:            // Sync jobs period
  status_update_period:   // PR status update period
  merge_method:           // Set to squash, rebase or just merge PR
  target_url:             // URL linked to from the details link on the Github status context
  pr_status_base_url:     // PR status page
  max_goroutines:         // Per-pool parallelism
```

Depending on your criteria, some of the items may by omitted.

See [https://godoc.org/github.com/kubernetes/test-infra/prow/config#Tide](https://godoc.org/github.com/kubernetes/test-infra/prow/config#Tide) for more detail.

### General configuration

The following configuration fields are available:

* `sync_period`: The field specifies how often Tide will sync jobs with Github. Defaults to 1m.
* `status_update_period`: The field specifies how often Tide will update Github status contexts.
   Defaults to the value of `sync_period`.
* `queries`: List of queries (described below).
* `merge_method`: A key/value pair of an `org/repo` as the key and merge method to override
   the default method of merge as value. Valid options are `squash`, `rebase`, and `merge`.
   Defaults to `merge`.
* `target_url`: URL for tide status contexts.
* `pr_status_base_url`: The base URL for the PR status page. If specified, this URL is used to construct
   a link that will be used for the tide status context. It is mutually exclusive with the `target_url` field.
* `max_goroutines`: The maximum number of goroutines spawned inside the component to
   handle org/repo:branch pools. Defaults to 20. Needs to be a positive number.

### Queries

The `queries` field specifies a list of queries.
Each query corresponds to a set of **open** PRs as candidates for merging.
It can consist of the following dictionary of fields:

* `repos`: List of queried repositories.
* `labels`: List of labels any given PR must posses.
* `missingLabels`: List of labels any given PR must not posses.
* `excludedBranches`: List of branches that get excluded when querying the `repos`.
* `includedBranches`: List of branches that get included when querying the `repos`.
* `reviewApprovedRequired`: If set, each PR in the query must have review approved.

Under the hood, a query constructed from the fields follows rules described in
https://help.github.com/articles/searching-issues-and-pull-requests/.
Therefore every query is just a structured definition of a standard GitHub
search query which can be used to list mergeable PRs.
The field to search token correspondence is based on the following mapping:

* `repos` -> `repo:org/repo`
* `labels` -> `label:lgtm`
* `missingLabels` -> `-label:do-not-merge`
* `excludedBranches` -> `-branch:dev`
* `includedBranches` -> `branch:master`
* `reviewApprovedRequired` -> `review:approved`

**Important**: Each query must return a different set of PRs. No two queries are allowed to contain the same PR.

Every PR that need to be rebased is filtered from the pool before processing


### Context Policy Options

A PR will be merged when all checks are passing. With this option you can customize
which contexts are required or optional.

By default, required and optional contexts will be derived from Prow Job Config.
This allows to find if required checks are missing from the github combined status.

If `branch-protection` config is defined, it can be used to know which test needs
be passing to merge a PR.

When branch protection is not used, required and optional contexts can be defined
globally, or at the org, repo or branch level.

If we want to skip unknown checks (ie checks that are not defined in Prow Config), we can set
`skip-unknown-contexts` to true. This option can be set globally or per org,
repo and branch.

**Important**: If this option is not set and no prow jobs are defined tide will trust the github
combined status and will assume that all checks are required (except tide).


### Example

```yaml
tide:
  merge_method:
    kubeflow/community: squash

  target_url: https://prow.k8s.io/tide.html

  queries:
  - repos:
    - kubeflow/community
    - kubeflow/examples
    labels:
    - lgtm
    - approved
    missingLabels:
    - do-not-merge
    - do-not-merge/hold
    - do-not-merge/work-in-progress
    - needs-ok-to-test
    - needs-rebase

  context_options:
    # Use branch protection options to define required and optional contexts
    from-branch-protection: true
    # Treat unknown contexts as optional
    skip-unknown-contexts: true
    orgs:
      org:
        required-contexts:
        - "check-required-for-all-repos"
        repos:
	  repo:
            required-contexts:
             - "check-required-for-all-branches"
            branches:
              branch:
                from-branch-protection: false
                required-contexts:
                - "required_test"
                optional-contexts:
                - "optional_test"

```

**Explanation**: The component starts periodically querying all PRs in `github.com/kubeflow/community` and
`github.com/kubeflow/examples` repositories that have `lgtm` and `approved` labels set
and do not have `do-not-merge`, `do-not-merge/hold`, `do-not-merge/work-in-progress`, `needs-ok-to-test` and `needs-rebase` labels set.
All PRs that conform to the criteria are processed and merged.
The processing itself can include running jobs (e.g. tests) to verify the PRs are good to go.
At the same time all commits in PRs from `github.com/kubeflow/community` repository are squashed before merging.

## Presubmits

Before a PR is merged, Tide ensures that all jobs configured as required (with `skip_report: false`) in the `presubmits` part of the `config.yaml` file
are passing against the latest base branch commit, rerunning the jobs if necessary.

Semantic of individual fields of the `presubmits` is described in [prow/README.md#how-to-add-new-jobs](https://github.com/kubernetes/test-infra/blob/master/prow/README.md#how-to-add-new-jobs).
