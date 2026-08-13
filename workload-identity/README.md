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

### Migrate Prow Job to Use Workload Identity

> **Note: Workload identity works best with pod utilities**: Migrate to use pod
> utilities if there is no `decoration: true` on your job, and come back to this
> doc once that's done.

#### Background

Prow jobs run in kubernetes pods, and each pod is responsible for uploading artifacts
to designated remote storage location(e.g. GCS), this upload is done by a
container called `sidecar`. To be able to upload to GCS, `sidecar` container
will need to way to authenticate to GCP, which was historically done by GCP
service account key(normally stored as `service-account` secret in build
clusters), this key is mounted onto `sidecar` container by prow, which it uses
for authenticating with GCP.

Workload identity is a keyless solution from GCP that appears to be more secure
than storing keys, and Prow also supports this by running the entire pod
with a service account that has GCS operation permission.

To migrate from using GCP service account keys to workload identity, there are
two different scenarios, and the steps are different. The differentiator is
whether the prow job itself directly interacts with GCP or not, if any of the
following config is in the job then it's very likely that the answer is yes:

```
volumes:
  - name: <DOES_NOT_MATTER>
    secret:
      secretName: service-account
```

```
labels:
  preset-service-account: "true"
```

#### Migration Steps when Not Interacting with GCP

Add the following sections on the prow job config:
```
decoration_config:
  gcs_credentials_secret: "" # Use workload identity for uploading artifacts
spec:
  # This account exists in "default" build cluster for now, for any other build cluster it
  # can be set up by following the steps from the top of this script.
  serviceAccountName: prowjob-default-sa
```

See [example PR of
migration](https://github.com/kubernetes/test-infra/pull/26374).

#### Migration Steps when Interacting with GCP

1. Inspect the test process/script invoked by the prow job, remove any logic
   that assumes the existence of GCP service account key file, or the
   environment variable of `GOOGLE_APPLICATION_CREDENTIALS` before migrating.

1. Creating new GCP service account with necessary GCP IAM permissions, and set
   up workload identity with the new service account by following [Kubernetes
   service account](#kubernetes-service-account) and [GCP service
   account](#gcp-service-account) above.

1. Modify prow job config by adding the sections similar to above, replacing the
   service account name with the new service account name.
