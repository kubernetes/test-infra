# Configuring Tide

Configuration of Tide is located under the [prow/config.yaml](/prow/config.yaml) file. All configuration for merge behavior and criteria belongs in the `tide` yaml struct, but it may be necessary to also configure presubmits for Tide to run against PRs (see ['Configuring Presubmit Jobs'](#configuring-presubmit-jobs) below).

This document will describe the fields of the `tide` configuration and how to populate them, but you can also check out the [GoDocs](https://godoc.org/github.com/kubernetes/test-infra/prow/config#Tide) for the most up to date configuration specification.

To deploy Tide for your organization or repository, please see [how to get started with prow](/prow/getting_started.md).

### General configuration

The following configuration fields are available:

* `sync_period`: The field specifies how often Tide will sync jobs with GitHub. Defaults to 1m.
* `status_update_period`: The field specifies how often Tide will update GitHub status contexts.
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

* `orgs`: List of queried organizations.
* `repos`: List of queried repositories.
* `labels`: List of labels any given PR must posses.
* `missingLabels`: List of labels any given PR must not posses.
* `excludedBranches`: List of branches that get excluded when querying the `repos`.
* `includedBranches`: List of branches that get included when querying the `repos`.
* `reviewApprovedRequired`: If set, each PR in the query must have at
  least one [approved GitHub pull request
  review](https://help.github.com/articles/about-pull-request-reviews/)
  present for merge. Defaults to `false`.

Under the hood, a query constructed from the fields follows rules described in
https://help.github.com/articles/searching-issues-and-pull-requests/.
Therefore every query is just a structured definition of a standard GitHub
search query which can be used to list mergeable PRs.
The field to search token correspondence is based on the following mapping:

* `orgs` -> `org:kubernetes`
* `repos` -> `repo:kubernetes/test-infra`
* `labels` -> `label:lgtm`
* `missingLabels` -> `-label:do-not-merge`
* `excludedBranches` -> `-branch:dev`
* `includedBranches` -> `branch:master`
* `reviewApprovedRequired` -> `review:approved`

**Important**: Each query must return a different set of PRs. No two queries are allowed to contain the same PR.

Every PR that needs to be rebased or is failing required statuses is filtered from the pool before processing


### Context Policy Options

A PR will be merged when all checks are passing. With this option you can customize
which contexts are required or optional.

By default, required and optional contexts will be derived from Prow Job Config.
This allows to find if required checks are missing from the GitHub combined status.

If `branch-protection` config is defined, it can be used to know which test needs
be passing to merge a PR.

When branch protection is not used, required and optional contexts can be defined
globally, or at the org, repo or branch level.

If we want to skip unknown checks (ie checks that are not defined in Prow Config), we can set
`skip-unknown-contexts` to true. This option can be set globally or per org,
repo and branch.

**Important**: If this option is not set and no prow jobs are defined tide will trust the GitHub
combined status and will assume that all checks are required (except for it's own `tide` status).


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
All commits in PRs from `github.com/kubeflow/community` repository are squashed before merging.

# Configuring Presubmit Jobs

Before a PR is merged, Tide ensures that all jobs configured as required in the `presubmits` part of the `config.yaml` file are passing against the latest base branch commit, rerunning the jobs if necessary. **No job is required to be configured** in which case it's enough if a PR meets all GitHub search criteria.

Semantic of individual fields of the `presubmits` is described in [prow/README.md#how-to-add-new-jobs](/prow/README.md#how-to-add-new-jobs).
