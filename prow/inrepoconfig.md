# Inrepoconfig

Inrepoconfig is a Prow feature that allows versioning Presubmit and Postsubmit
jobs in the same repository that also holds the code (with a `.prow` directory
or `.prow.yaml` file, akin to a `.travis.yaml` file). So instead of having all
your jobs defined centrally, you could instead define the jobs in a distributed
manner, coupled closely with the source code repos that they work on.

If enabled, Prow will use both the centrally-defined jobs and the ones defined
in the code repositories. The latter ones are dynamically loaded on-demand.

## Why use Inrepoconfig?

### Pros

- Coupling the jobs with the source code allows you to update both the job and
  the source code at the same time.

### Cons

- Inrepoconfig jobs are loaded on-demand, so it takes some extra setup to check
  that a misconfigured Inrepoconfig job is not blocking a PR. See "Config
  verification job" below.

## Basic usage

To enable it, add the following to your Prow's `config.yaml`:

```
in_repo_config:
  enabled:
    # The key can be one of "*" for "globally", "org" or "org/repo".
    # The narrowest match is used. Here the key is "kubernetes/kubernetes".
    kubernetes/kubernetes: true

  # Clusters must be allowed before they can be used. Here we allow the "default"
  # cluster globally. This setting also allows using "*" for "globally", "org" or "org/repo" as key.
  # All clusters that are allowed for the specific repo, its org or
  # globally can be used.
  allowed_clusters:
    "*": ["default"]
```

Additionally, `Deck` must be configured with a GitHub token if that is not already the case. To do
so, the `--github-token-path=` flag must be set and point to a valid token file that has permissions
to read all your repositories. Also, in order for Deck to serve content from storage locations not
defined in the default locations or centrally-defined jobs, those buckets must be listed
in `deck.additional_allowed_buckets`.

### Config verification job

Afterwards, you need to add a config verification job to make sure people people get told about
mistakes in their Inrepoconfig rather than the PR being stuck. It makes sense to define this
job in the central repository rather than the code repository, so the `checkconfig` version used
stays in sync with the Prow version used. It looks like this:

```
presubmits:
  kubernetes/kubernetes:
  - name: pull-kubernetes-validate-prow-yaml
    always_run: true
    decorate: true
    extra_refs:
    - org: kubernetes
      repo: test-infra
      base_ref: master
    spec:
      containers:
      - image: gcr.io/k8s-prow/checkconfig:v20191205-050b151d0
        command:
        - /app/prow/cmd/checkconfig/app.binary
        args:
        - --plugin-config=../test-infra/path/to/plugins.yaml
        - --config-path=../test-infra/path/to/config.yaml
        - --prow-yaml-repo-name=$(REPO_OWNER)/$(REPO_NAME)
```

After deploying the new config, the only step left is to create jobs. This is done by adding a file
named `.prow.yaml` to the root of the repository that holds your code:

```yaml
presubmits:
- name: pull-test-infra-yamllint
  always_run: true
  decorate: true
  spec:
    containers:
    - image: quay.io/kubermatic/yamllint:0.1
      command:
      - yamllint
      - -c
      - config/jobs/.yamllint.conf
      - config/jobs
      - config/prow/cluster

postsubmits:
- name: push-test-infra-yamllint
  always_run: true
  decorate: true
  spec:
    containers:
    - image: quay.io/kubermatic/yamllint:0.1
      command:
      - yamllint
      - -c
      - config/jobs/.yamllint.conf
      - config/jobs
      - config/prow/cluster
```

## Multiple config files

It is possible also to use multiple config files with this same format under a `.prow`
directory in the root of your repo. All the YAML files under the `.prow` directory will
be read and merged together recursively. This makes it easier to handle big repos with
a large number of jobs and allows fine-grained OWNERS control on them.

The `.prow` directory and `.prow.yaml` file are mutually exclusive; when both are present the `.prow` directory takes precedence.

For more detailed documentation of possible configuration parameters for jobs, please check the [job documentation](/prow/jobs.md)
