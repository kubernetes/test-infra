# Tide Documentation

Tide merges PR that match a given sets of criteria

## Tide configuration

Extend the primary prow [`config.yaml`] document to include a top-level
`tide` key that looks like the following:

```yaml

tide:
  queries:
  ...
  merge_method:
  ...


presubmits:
  kubernetes/test-infra:
  - name: fancy-job-name
    context: fancy-job-name
    always_run: true
    spec:  # podspec that runs job
```


### Merging Options

Tide supports all 3 github merging options:

* squash
* merge
* rebase

A merge method can be set for repo or per org.

Example:

```yaml
tide:
  ...
  merge_method:
    org1: squash
    org2/repo1: rebase
    org2/repo2: merge
```

### Queries Configuration

Queries are using github queries to find PRs to be merged. Multiple queries can be defined for a single repo. Queries support filtering per existing and missing labels. In order to filter PRs that have been approved, use the reviewApprovedRequired.

```yaml
tide:
  queries:
    ...
    - repos:
      - org1/repo1
      - org2/repo2
      labels:
      - labelThatMustsExists
      - OtherLabelThatMustsExist
      missingLabels:
      - labelThatShouldNotBePresent
     # If you want github approval
     reviewApprovedRequired: true
```

