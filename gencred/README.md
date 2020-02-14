# Gencred

> **NOTE:** the user who runs `gencred` must have a [`cluster-admin`] role (or it will
> fail with a privilege escalation error).

## Create a new context

The following creates a `new-context` in a ~/new-kubeconfig.yaml file that authorizes as a cluster-admin in `target-context`:

```bash
# go run k8s.io/test-infra/gencred <options>
# See --help for current list of options
bazel run //gencred -- --context=target-context --name=new-context --output ~/new-kubeconfig.yaml --serviceaccount
```

The resulting `new-kubeconfig.yaml` file will look something like:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: same-as-target-context-cluster-info
    server: https://example.com
  name: new-context
contexts:
- context:
    cluster: new-context
    user: new-context
  name: new-context
kind: Config
preferences: {}
users:
- name: new-context
  user:
    token: fake-token
```

The most relevant parts here are `token:` and `certificate-authority-data:`.

Validate things work with the following command:

```bash
KUBECONFIG=$HOME/new-kubeconfig.yaml kubectl --context=new-context get pods
```

## Add to prow cluster

1) Add this file to a secret that prow can access (and/or merge the values into
the existing one).  *TODO(fejta): provide tooling and/or example commands for doing this reliably*
  * Best practice is to use a different secret or file within the secret

2) Ensure prow binaries load kubeconfig file ([example flag usage]).

3) Add `cluster: new-context` to jobs that should schedule in this cluster
([example job usage]).


[`cluster-admin`]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles
[example flag usage]: https://github.com/GoogleCloudPlatform/oss-test-infra/blob/537ffbfde85b0579857807b9f6dd70b9a25bf0b0/prow/cluster/cluster.yaml#L143-L167
[example job usage]: https://github.com/kubernetes/test-infra/blob/96cadf7b32ecd3c0d2c38a870e3347d4b98873b8/config/jobs/kubernetes/sig-scalability/sig-scalability-release-blocking-jobs.yaml#L5
