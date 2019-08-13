# Branchcleaner

The `branchcleaner` plugin automatically deletes source branches for merged PRs between two branches
on the same repository. This is helpful to keep repos that don't allow forking clean.

## Usage

Enable the `branchcleaner` in the desired repos via the `plugins.yaml`:

```
plugins:
  org/repo:
  - branchcleaner
```
