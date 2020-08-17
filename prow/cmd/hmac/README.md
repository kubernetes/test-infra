# HMAC

`hmac` is a tool to update the HMAC token, GitHub webhooks and HMAC secret
for the orgs/repos as per the `managed_webhooks` configuration changes in the Prow config file.

## Prerequisites

To run this tool, you'll need:

1. A github account that has admin permission to the orgs/repos.

1. A [personal access token](https://github.com/settings/tokens) for the github account. 
Note the token must be granted `admin:repo_hook` and ` admin:org_hook` scopes.

1. Permissions to read&write the hmac secret in the Prow cluster.

## How to run this tool

There are two ways to run this tool:

1. Run it on local:

```sh
bazel run //prow/cmd/hmac -- \
  --config-path=/path/to/prow/config \
  --github-token-path=/path/to/oauth/secret \
  --kubeconfig=/path/to/kubeconfig \
  --kubeconfig-context=[context of the cluster to connect] \
  --hmac-token-secret-name=[hmac secret name in Prow cluster] \
  --hmac-token-key=[key of the hmac tokens in the secret] \
  --hook-url http://an.ip.addr.ess/hook \
  --dryrun=true  # Remove it to actually update hmac tokens and webhooks
```

2. Run it as a Prow job:

The recommended way to run this tool would be running it as a postsubmit job.
One example Prow job configured for k8s Prow can be found [here](https://github.com/kubernetes/test-infra/blob/b11722064aea0913f4b02cb6aabda1f91f0abc7f/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L113-L156).

## How it works

Given a new `managed_webhooks` configuration in the Prow core config file,
the tool can reconcile the current state of HMAC tokens, secrets and
webhooks to meet the new configuration.

### Configuration example

Below is a typical example for the managed_webhooks configuration:

```yaml
managed_webhooks:
  # Whether this tool should respect the legacy global token.
  # This has to be true if any of the managed repo/org is using the legacy global token that is manually created.   
  respect_legacy_global_token: true
  # Config for orgs and repos that have been onboarded to this Prow instance.  
  org_repo_config:
    qux:
      token_created_after: 2017-10-02T15:00:00Z
    foo/bar:
      token_created_after: 2018-10-02T15:00:00Z
    foo/baz:
      token_created_after: 2019-10-02T15:00:00Z
```

### Workflow example

Suppose the current `org_repo_config` in the `managed_webhooks` configuration is
```yaml
qux:
  token_created_after: 2017-10-02T15:00:00Z
foo/bar:
  token_created_after: 2018-10-02T15:00:00Z
foo/baz:
  token_created_after: 2019-10-02T15:00:00Z
``` 

There can be 3 scenarios to modify the configuration, as explained below:

#### Rotate an existing HMAC token

User updates the `token_created_after` for `foo/baz` to a later time, as shown below:
```yaml
qux:
  token_created_after: 2017-10-02T15:00:00Z
foo/bar:
  token_created_after: 2018-10-02T15:00:00Z
foo/baz:
  token_created_after: 2020-03-02T15:00:00Z
``` 

The `hmac` tool will generate a new HMAC token for the `foo/baz` repo,
add the new token to the secret, and update the webhook for the repo.
And after the update finishes, it will delete the old token.

#### Onboard a new repo

User adds a new repo `foo/bax` in the `managed_webhooks` configuration, as shown below:
```yaml
qux:
  token_created_after: 2017-10-02T15:00:00Z
foo/bar:
  token_created_after: 2018-10-02T15:00:00Z
foo/baz:
  token_created_after: 2019-10-02T15:00:00Z
foo/bax:
  token_created_after: 2020-03-02T15:00:00Z
``` 

The `hmac` tool will generate an HMAC token for the `foo/bax` repo,
add the token to the secret, and add the webhook for the repo.

#### Remove an existing repo

User deletes the repo `foo/baz` from the `managed_webhooks` configuration, as shown below:
```yaml
qux:
  token_created_after: 2017-10-02T15:00:00Z
foo/bar:
  token_created_after: 2018-10-02T15:00:00Z
``` 

The `hmac` tool will delete the HMAC token for the `foo/baz` repo from
the secret, and delete the corresponding webhook for this repo.

> Note the 3 types of config changes can happen together, and `hmac` tool
> is able to handle all the changes in one single run.
