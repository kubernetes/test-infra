# Configuring Tide

Configuration of Tide is located under the [config/prow/config.yaml](/config/prow/config.yaml) file. All configuration for merge behavior and criteria belongs in the `tide` yaml struct, but it may be necessary to also configure presubmits for Tide to run against PRs (see ['Configuring Presubmit Jobs'](#configuring-presubmit-jobs) below).

This document will describe the fields of the `tide` configuration and how to populate them, but you can also check out the [GoDocs](https://godoc.org/github.com/kubernetes/test-infra/prow/config#Tide) for the most up to date configuration specification.

To deploy Tide for your organization or repository, please see [how to get started with prow](/prow/getting_started_deploy.md).

### General configuration

The following configuration fields are available:

* `batch_allow_pending`: Boolean that allows tide to pick PRs for batch that have pending tests.
* `sync_period`: The field specifies how often Tide will sync jobs with GitHub. Defaults to 1m.
* `status_update_period`: The field specifies how often Tide will update GitHub status contexts.
   Defaults to the value of `sync_period`.
* `queries`: List of queries (described below).
* `merge_method`: A key/value pair of an `org/repo` as the key and merge method to override
   the default method of merge as value. Valid options are `squash`, `rebase`, and `merge`.
   Defaults to `merge`.
* `merge_commit_template`: A mapping from `org/repo` or `org` to a set of Go templates to use when creating the title and body of merge commits. Go templates are evaluated with a `PullRequest`  (see [`PullRequest`](https://godoc.org/k8s.io/test-infra/prow/tide#PullRequest) type). This field and map keys are optional.
* `target_url`: URL for tide status contexts.
* `pr_status_base_url`: The base URL for the PR status page. If specified, this URL is used to construct
   a link that will be used for the tide status context. It is mutually exclusive with the `target_url` field.
* `max_goroutines`: The maximum number of goroutines spawned inside the component to
   handle org/repo:branch pools. Defaults to 20. Needs to be a positive number.
* `blocker_label`: The label used to identify issues which block merges to repository branches.
* `squash_label`: The label used to ask Tide to use the squash method when merging the labeled PR.
* `rebase_label`: The label used to ask Tide to use the rebase method when merging the labeled PR.
* `merge_label`: The label used to ask Tide to use the merge method when merging the labeled PR.

### Merge Blocker Issues

Tide supports temporary holds on merging into branches via the `blocker_label` configuration option.
In order to use this option, set the `blocker_label` configuration option for the Tide deployment.
Then, when blocking merges is required, if an open issue is found with the label it will block merges to
all branches for the repo. In order to scope the branches which are blocked, add a `branch:name` token
to the issue title. These tokens can be repeated to select multiple branches and the tokens also support
quoting, so `branch:"name"` will block the `name` branch just as `branch:name` would.

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
* `author`: The author of the PR.
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
* `author` -> `author:batman`
* `reviewApprovedRequired` -> `review:approved`


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

  target_url: https://prow.k8s.io/tide

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

### Persistent Storage of Action History

Tide records a history of the actions it takes (namely triggering tests and merging).
This history is stored in memory, but can be loaded from GCS and periodically flushed
in order to persist across pod restarts. Persisting action history to GCS is strictly
optional, but is nice to have if the Tide instance is restarted frequently or if
users want to view older history.

Both the `--history-uri` and `--gcs-credentials-file` flags must be specified to Tide
to persist history to GCS. The GCS credentials file should be a [GCP service account
key](https://cloud.google.com/iam/docs/service-accounts#service_account_keys) file
for a service account that has permission to read and write the history GCS object.
The history URI is the GCS object path at which the history data is stored. It should
not be publicly readable if any repos are sensitive and must be a GCS URI like `gs://bucket/path/to/object`.

[Example](https://github.com/kubernetes/test-infra/blob/b4089633afbe608271a6630bb66c6d74f29f78ef/prow/cluster/tide_deployment.yaml#L40-L41)

# Configuring Presubmit Jobs

Before a PR is merged, Tide ensures that all jobs configured as required in the `presubmits` part of the `config.yaml` file are passing against the latest base branch commit, rerunning the jobs if necessary. **No job is required to be configured** in which case it's enough if a PR meets all GitHub search criteria.

Semantic of individual fields of the `presubmits` is described in [prow/jobs.md](/prow/jobs.md).
