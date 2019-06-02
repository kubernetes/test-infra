# Inrepoconfig

The `inrepoconfig` extension allows to configure Presubmits inside the repository
that is being tested in addition to configure them in a central repository that
holds all of the `Prow` configuration.

## Usage


Make sure the `trigger` plugin is activated. Afterwards, enable `inrepoconfig`
inside your Prows main `config.yaml` with a block like this:

```yaml
in_repo_config:
  '*':
    enabled: true
  'my-special-org':
    enabled: false
  'my-normal-org/my-special-repo':
    enabled: false
```

The above sample enables `inrepoconfig` for all repos, except for the ones that
are in the GitHub organization `my-special-org` or for the
`my-normal-org/my-special-repo` repository. You can combine the `*`, `org`,
`org/repo` keys as you wish, the narrowest match will always take precedence.

Afterwards, you can add a `prow.yaml` file to any of your repositories. The
structure is as follow:

```yaml
presubmits:
- name: pull-test-inrepo-hello
  always_run: true
  decorate: true
  spec:
    containers:
    - image: alpine
      command:
      - echo hello world
```
