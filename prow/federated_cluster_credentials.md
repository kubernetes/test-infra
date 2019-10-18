# Federating Kubernetes Clusters for Prow

Prow supports using a federated set of Kubernetes clusters for deployment. This feature enables two major use-cases:

 - creating resources to execute ProwJobs across multiple environments
 - incrementally migration Prow infrastructure services from one cluster to another

This document explains how to opt into a federated deployment topology and how to configure which resources exist in which clusters.

### Definitions
 
Before diving into how this feature is configured and used, the following definitions need to be made:

| Term | Definition |
| ---- | ---------- |
| _cluster_ | A _cluster_ is a conformant Kubernetes deployment. _clusters_ are specified in a `~/.kube/config` file. |
| `cluster` | The `cluster` field on a ProwJob refers to the _context_ in which resources are created to execute it. |
| _context_ | A _context_ is an identifier for a credential, specifying a _cluster_, a `Namespace` on that cluster, and a _user_ whose credentials should be used when interacting. _contexts_ are specified in a `~/.kube/config` file. |
| _user_    | A _user_ is an identifier for a Kubernetes identity; _users_ are specified in a `~/.kube/config` file. |
| _service cluster_ | The _service cluster_ is the _context_ in which the `ProwJob` `CustomResourceDefinition` is deployed. |
| _in-cluster context_ | The _in-cluster context_ is a _context_ for the `ServiceAccount` under which a `Pod` runs, loaded from a customary location in the `Pod`'s filesystem. |

### Flags

All Prow controllers that interact with resources expose the following flags:

| Flag | Use |
| ---- | --- |
| `--kubeconfig` | Path to a `~/.kube/config` file to load. Can be passed more than once. |
| `--use-explicit-credentials` | A feature flag to opt into the behavior in this document. |
| `--prow-job-context` | The _context_ for the _service cluster_, used to interact with `ProwJob` `CustomResources`. |
 
Note: until January 2020, the behavior in this document will be guarded behind the `--use-explicit-credentials` flag.

## Configuring Credentials

In order to configure federated credentials, Prow uses the `Config` format (often stored in `~/.kube/config`). Multiple `Config` files may be provided by passing `--kubeconfig` more than once. These files will be merged with a simple union on the _clusters_, _contexts_ and _users_ in each. If any names collide, loading will fail.

## Defaulting

When the `--kubeconfig` flag is not passed, Prow will use the _in-cluster context_ for all operations, like:

 - creating and reading `ProwJob` `CustomResources`
 - running `Pods` to execute `ProwJob`s that use the `kubernetes` agent
 - executing Tekton pipelines for `ProwJob`s that use the `tekton` agent
 
If a `--kubeconfig` flag is passed and the `--use-explicit-credentials` flag is set, no defaulting of any sort will occur. No _context_ is default and no _context_ is special in this mode. Jobs must opt into using a specific loaded _context_ by setting the `cluster` field. Prow components will continue using the _in-cluster context_ for interacting with `ProwJob` `CustomResources`.

## Specifying an _in-cluster context_ via `--kubeconfig`

Prow will post-process the `Config` loaded via `--kubeconfig` to allow users to specify the _in-cluster context_ but pass credentials by setting a `ServiceAccount` for the `Pod` in question. 

In order to opt into this, use the `"in-cluster"` string in a _context_ and omit a _user_:

```yaml
contexts:
- name: "my-in-cluster-context"
  context:
    cluster: "in-cluster"
    namespace: "default"
```

## A Note On Namespaces

Although the field used to target a _context_ for a `ProwJob` is named `cluster`, in reality the field targets a _cluster_ in a specific `Namespace` using a specific _user_. In keeping with Kubernetes' multi-tenant nature, Prow resources are namespaced and everything from the _service cluster_ to any given job may be sequestered in a unique `Namespace`. 

An administrator may allow scheduling `Pod`s by `plank` in a specific `Namespace` by:

 - creating a `ServiceAccount` with the required `Role`s and `RoleBinding`s to allow for `Pod` operations
 - adding the `ServiceAccount` as a _user_ to the `Config`, providing the OAuth token for credentials
 - adding a _context_ to target the appropriate _cluster_ and `Namespace` with this new _user_

The `namespace` field on a _context_ simply sets the default `Namespace` for clients, but is a convenient means for communicating the correct `Namespace` for a client which will have rights to create resources in only one `Namespace` from the RBAC that applies to it.

## Migrating From Previous Configurations

Until January 2020, the features in this document are not the default behavior for Prow clusters with `--kubeconfig` set; `--use-explicit-credentials` must be set to opt in.

When migrating from a previous configuration (build cluster YAML or implicit `Config`), apply the following steps:

 - create a `Config` file with all _contexts_ previously present, making sure to explicitly specify the `Namespace` in which resources should be created
 - ensure all jobs set the `cluster` field (in the past they may have implicitly opted into using the `"default"` _context_, if one existed, by not setting this field at all)
 - set the `--use-explicit-credentials` flag
 - remove build cluster YAML if present
 - unset the `prowjob_namespace` and `pod_namespace` fields in the main Prow `config.yaml`
 
## Migrating Prow Services

In some cases it may be necessary to migrate the _service cluster_. In order to do this in a minimally impactful manner, it is possible to configure any individual Prow service to interact with `ProwJob` `CustomResources` in a _context_ different from the _in-cluster context_. To do this:
 
 - add a _user_ to the `Config` for the `ServiceAccount` used by the service being migrated
 - add a _context_ to the `Config` which points to the _cluster_ currently hosting `ProwJob` `CustomResource`s and uses the new _user_
 - create a `Deployment` of the service being migrated in a new _cluster_ which loads the `Config` and sets `--prow-job-context` to the _context_ created above
 
Note: Once all services have been migrated, it will be necessary to migrate the `ProwJob` `CustomResource`s themselves. This has not yet been implemented; contact @stevekuznetsov on GitHub or Slack for more information.

## Examples

### No Federation

#### Flags and Configuration

No `--kubeconfig` flag is provided. No job configuration is changed.

#### Behavior

 - Prow services will use the _in-cluster context_ as the _service cluster_ for interacting with `ProwJob` `CustomResource`s.
 - Prow services will use the _in-cluster context_ as the _context_ for Kubernetes resources necessary for executing `ProwJob`s.
 - `ProwJob`s may not set the `cluster` field.

### Federated Jobs Scheduling Into A Custom Namespace

#### Flags and Configuration

The `--kubeconfig` flag provides the following `Config`:

```yaml
clusters: []
contexts:
- name: "default"
  context:
    cluster: "in-cluster"
    namespace: "default"
- name: "custom-namespace"
  context:
    cluster: "in-cluster"
    namespace: "something-custom"
users: []
```

A new job is configured like:

```yaml
periodics:
- name: "new-test"
  cluster: "custom-namespace"
  interval: "10m"
  decorate: true
  spec:
    containers:
    - image: "alpine"
      command: ["/bin/date"]
```

Existing jobs change to be configured like:

```diff
 periodics:
 - name: "existing-test"
+  cluster: "default"
   interval: "10m"
   decorate: true
   spec:
     containers:
     - image: "alpine"
       command: ["/bin/date"]
```

#### Behavior

 - Prow services will use the _in-cluster context_ as the _service cluster_ for interacting with `ProwJob` `CustomResource`s.
 - Prow services will use the _context_ specified in the `cluster` field for Kubernetes resources necessary for executing `ProwJob`s. The `"default"` _context_ and the `"custom-namespace"` _context_ both load _in-cluster_ credentials.  
 - `ProwJob`s must set the `cluster` field.

### Federated Jobs Scheduling Into A Separate Cluster

#### Flags and Configuration

The `--kubeconfig` flag provides the following `Config`:

```yaml
clusters:
- name: "my-build-cluster"
  cluster:
    server: "http.buildme.com"
contexts:
- name: "default"
  context:
    cluster: "in-cluster"
    namespace: "default"
- name: "custom-cluster"
  context:
    cluster: "my-build-cluster"
    namespace: "something-custom"
    user: "my-service-account"
users:
- name: "my-service-account"
  user:
    token: "long-hash-here"
```

A new job is configured like:

```yaml
periodics:
- name: "new-test"
  cluster: "custom-cluster"
  interval: "10m"
  decorate: true
  spec:
    containers:
    - image: "alpine"
      command: ["/bin/date"]
```

Existing jobs change to be configured like:

```diff
 periodics:
 - name: "existing-test"
+  cluster: "default"
   interval: "10m"
   decorate: true
   spec:
     containers:
     - image: "alpine"
       command: ["/bin/date"]
```

#### Behavior

 - Prow services will use the _in-cluster context_ as the _service cluster_ for interacting with `ProwJob` `CustomResource`s.
 - Prow services will use the _context_ specified in the `cluster` field for Kubernetes resources necessary for executing `ProwJob`s. The `"default"` _context_ loads _in-cluster_ credentials; the `"custom-cluster"` _context_ uses static credentials presented in the `Config`.
 - `ProwJob`s must set the `cluster` field.
