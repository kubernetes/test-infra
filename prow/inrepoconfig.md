# Inrepoconfig

Inrepoconfig is a Prow feature that allows versioning Presubmit and Postsubmit jobs in the same repository
that also holds the code. If enabled, Prow will use both the centrally-defined jobs and the
ones defined in the code repository. The latter ones are dynamically loaded on-demand.

To enable it, add the following to your Prows `config.yaml`:

```
in_repo_config:
  enabled:
    # The key can be one of "*" for "globally", "org" or "org/repo".
    # The narrowest match is used.
    kubernetes/kubernetes: true

  # Clusters must be allowed before they can be used. Below is the default: Allow the `default` cluster
  # globally.
  # This setting also allows using "*" for "globally", "org" or "org/repo" as key.
	# a given repo. All clusters that are allowed for the specific repo, its org or
	# globally can be used.
  allowed_clusters:
    "*": ["default"]
```

Additionally, `Deck` must be configured with an oauth token if that is not already the case. To do
so, the `--github-token-path=` flag must be set and point to a valid token file that has permissions
to read all your repositories. Also, in order for Deck to serve content from storage locations not
defined in the default locations or centrally-defined jobs, those buckets must be listed 
in `deck.additional_allowed_buckets`.

Afterwards, you need to add a config verification job to make sure people people get told about
mistakes in their `.prow.yaml` rather than the PR being stuck. It makes sense to define this
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

For a more detailed documentation of possible configuration parameters for jobs, please check the [job documentation](/prow/jobs.md)
