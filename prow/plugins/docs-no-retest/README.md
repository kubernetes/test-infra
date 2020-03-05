# Docs-No-Retest

`docs-no-retest` is a Prow Plugin that manages a `retest-not-required-docs-only` label. This indicates
whether a given pull requests only changes documentation.  In these cases it would not need to be retested.

## Deprecation

This plugin only works with `mungegithub` and has been slated for deletion after March 31, 2020. Avoid adding this plugin to your Prow instance.

If you want a test to only run when certain files are changed (such as non-documentation), add a line like this to the Prow Job:

```yaml
    run_if_changed: '.*.go|.*.yaml' # Or any path regex
```
