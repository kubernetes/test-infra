# config-forker

config-forker forks presubmit, periodic, and postsubmit job configs with the `fork-per-release` annotation.

## Usage

* `--job-config`: Path to the job config
* `--output`: Path to the output yaml to create. If not specified, the process still runs but no file is generated
  (potentially useful for presubmits)
* `--version`: the version to generate a forked config for, e.g. `1.15`

## Supported annotations

- `fork-per-release`: only jobs with this set to `"true"` will be forked.
- `fork-per-release-replacements`: allows replacing values in job `tags` (periodics only) and container `args` (see [Custom replacements](#custom-replacements)).
- `fork-per-release-deletions`: allows deleting values in job `labels` in periodics (see [Custom deletions](#custom-deletions)).
- `fork-per-release-periodic-interval`: if set, forked jobs will use this value for `interval`. If multiple space-separated values are provided, the first will be used.
- `fork-per-release-cron`: if set, forked jobs will use this value for `cron`. If multiple values separated with `, ` are provided, the first will be used.

## Actions taken

For the sake of clarity, all the below will assume we are forking with `--version 1.15`.

Only jobs annotated with `fork-per-release` will be forked.

For all jobs:

- If `spec` includes a container with an `image` ending in `-master`, it is replaced with `-1.15`.
- If `spec` includes a container with an `env` variable with `BRANCH` (case-insensitive) in the name and the value
  `master`, the value will be changed to `release-1.15`.
- If the `fork-per-release-replacements` annotation is specified, those replacements will be performed in the `args`
  of all containers for that job.
- If the `testgrid-dashboards` annotation is specified, references to `master-blocking` and `master-informing` are
  changed to `1.15-blocking` and `1.15-informing`.
- If the `testgrid-tab-name` annotation is specified, references to `master` are changed to `1.15`.
- If the `description` annotation is specified, it is removed (for now).

For presubmits and postsubmits:

- `skip_branches` will be deleted
- `branches` will be set to `release-1.15`

For periodics and postsubmits:

- If the job `name` ends in `-master`, it will be replaced with `-1-15`, otherwise `-1-15` will be appended
- `sig-release-1.15-all` is added to the job's `testgrid-dashboards` annotation (creating the annotation if necessary)

For periodics only:

- If `decorate` is `true`, and `extra_refs` contains a reference to `kubernetes/kubernetes` master, the `BaseRef`
  will be updated to `release-1.15`
- If `decorate` is not true, some bootstrap-related `args` will be replaced:
  - `--repo=k8s.io/kubernetes` or `--repo=k8s.io/kubernetes=master` will be replaced with `--repo=k8s.io/kubernetes=release-1.15`
  - `--branch=master` will be replaced with `--branch=release-1.15`
- If the `fork-per-release-periodic-interval` annotation is set, the `interval` will be set to the first value it contains.
 
## Custom replacements

The `fork-per-release-replacements` annotation can be used for custom replacements in your `args` or `tags` (periodic jobs only).
It takes the form of a comma-separated list of replacements, like this:

```
fork-per-release-replacements: "original1 -> replacement1, original2 -> replacement2"
```

The replacements are interpreted as Go templates, with one value defined: `{{.Version}}`. `{{.Version}}` will be replaced
by the version currently being forked (e.g. `1.15`). For example:

```
fork-per-release-replacements: "--version=stable -> --version={{.Version}"
```

## Custom deletions

The `fork-per-release-deletions` annotation can be used for custom deletions in your `labels` (periodic jobs only).
This is a comma-separated list of keys of labels you would like to remove on forking, for instance:

```
fork-per-release-deletions: "label-key-to-delete1, label-key-to-delete2"
```

This is useful for getting rid of a specific preset in the forked job. For instance, one can have a master branch job that
has a label corresponding to a master-specific preset which is undesired for forked release jobs.
