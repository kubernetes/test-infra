# artifact-uploader

The artifact-uploader image watches ProwJobs pods and uploads pods logs when pods teminate.

## Configuration

`artifact-uploader` accepts the following command line arguments :

| name | description | default value | mandatory |
| --- | --- | --- | --- |
| `num-workers` | Number of threads to use for processing updates | 25 | No |
| `prow-job-ns` | Namespace containing ProwJobs | "" | Yes |

In addtition, it accepts the same command-line arguments as [gcsupload](/prow/cmd/gcsupload).

## Deployment

A kubernetes `Deployment` example looks like this (`<namespace>` being the namespace where `artifact-uploader` is deployed and `<prow-jobs-namespace>` the namespace containing ProwJobs) :

```yaml
kind: ServiceAccount
apiVersion: v1
metadata:
  name: artifact-uploader
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: artifact-uploader
rules:
  - apiGroups:
      - prow.k8s.io
    resources:
      - prowjobs
    verbs:
      - get
      - watch
      - list
      - patch
  - apiGroups:
      - ''
    resources:
      - pods/log
    verbs:
      - get
  - apiGroups:
      - ''
    resources:
      - pods
    verbs:
      - list
      - watch
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: artifact-uploader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: artifact-uploader
subjects:
  - kind: ServiceAccount
    name: artifact-uploader
    namespace: <namespace>
---
kind: Deployment
apiVersion: extensions/v1beta1
metadata:
  name: artifact-uploader
  labels:
    app: artifact-uploader
spec:
  replicas: 1
  selector:
    matchLabels:
      app: artifact-uploader
  template:
    metadata:
      labels:
        app: artifact-uploader
    spec:
      containers:
      - name: artifact-uploader
        image: gcr.io/k8s-prow/artifact-uploader:latest
        args:
        - "--prow-job-ns=<prow-jobs-namespace>"
      serviceAccountName: artifact-uploader
```

Apply the above manifest with `kubectl apply -n <namespace> -f manifest.yaml`.

## Building

The gcr.io/k8s-prow/artifact-uploader image is built and published automatically by [`post-test-infra-push-prow`](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L104-L123) with the rest of the Prow components.

You can build the image locally with `bazel run //prow/cmd/artifact-uploader:image` (note `bazel run` not `bazel build`).
Publish to a remote repository after building with `docker push` or build and push all Prow images at once with [`prow/push.sh`](/prow/push.sh).
