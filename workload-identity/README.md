# Overview

Workload identity is the best practice for authenticating as a service account
when running on GKE.

See https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity for
how this works

## Configuration
### Cluster and node pools

First enable workload identity on the cluster and all node-pools in the cluster:

```bash
enable-workload-identity.sh K8S_PROJECT ZONE CLUSTER
```

### Kubernetes service account

Next ensure the kubernetes service account exists and that it has an
`iam.gke.io/gcp-service-account` annotation. This associates it with the desired
GCP service account.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations:
    iam.gke.io/gcp-service-account: SOMEBODY@PROJECT.iam.gserviceaccount.com
  name: SOMETHING
  namespace: SOMEWHERE
```

### GCP service account

Once this service account exists in the cluster, then an owner of
`SOMEBODY@PROJECT.iam.gserviceaccount.com` -- typically a `PROJECT` owner --
runs `bind-service-accounts.sh`:

```bash
bind-service-accounts.sh \
  K8S_PROJECT ZONE CLUSTER SOMEWHERE SOMETHING \
  SOMEBODY@PROJECT.iam.gserviceaccount.com
```

This script assumes the same person can access both `K8S_PROJECT` and
`PROJECT`. If that is not true then the `PROJECT` owner can just run this
command directly:

```bash
# Note: K8S_PROJECT is the project owning the GKE cluster
#       whereas PROJECT owns the service account (may be the same)
gcloud iam service-accounts add-iam-policy-binding \
  --project=PROJECT \
  --role=roles/iam.workloadIdentityUser \
  --member=serviceAccount:K8S_PROJECT.svc.id.goog[SOMEWHERE/SOMETHING] \
  SOMEBODY@PROJECT.iam.gserviceaccount.com
```

This is what tells GCP that the `SOMEWHERE/SOMETHING` service account in
`K8S_PROJECT` is authorized to act as
`SOMEBODY@PROJECT.iam.gserviceaccount.com`.

These are all described in the how-to GKE doc link at the top.

### Pods

At this point any pod that:
* Runs in a `K8S_PROJECT` GKE cluster
* Inside the `SOMEWHERE` namespace
* Using the `SOMETHING` service

will authenticate as `SOMEBODY@PROJECT.iam.gserviceaccount.com` to GCP. The
`bind-service-accounts.sh` script will verify this (see the how-to doc above for
the manual command).

Whenever you want a pod to authenticate this way, just configure the
`serviceAccountName` on the `PodSpec`. See an example
[pod](https://github.com/GoogleCloudPlatform/testgrid/blob/5c7bc80b18ccf00c773c34583628091890b401ab/cluster/summarizer_deployment.yaml#L22)
and
[prowjob](https://github.com/GoogleCloudPlatform/oss-test-infra/blob/9c466c14e4b4b5fbc8c837d0c61c779194e82d56/prow/prowjobs/GoogleCloudPlatform/oss-test-infra/gcp-oss-test-infra-config.yaml#L65):

Here are minimal deployment and prow job that use an image and args to print the
authenticated user to `STDOUT`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  namespace: SOMEWHERE # from above
  labels:
    app: foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: foo
  template:
    metadata:
      labels:
        app: foo
    spec:
      serviceAccountName: SOMETHING # from above
      containers:
      - name: whatever
        image: google/cloud-sdk:slim
        args:
        - gcloud
        - auth
        - list
```

```yaml
periodics:
- name: foo
  interval: 10m
  decorate: true
  spec:
    serviceAccountName: SOMETHING # from above, note: namespace is chosen by prow
    containers:
    - image: google/cloud-sdk:slim
      args:
      - gcloud
      - auth
      - list
```
