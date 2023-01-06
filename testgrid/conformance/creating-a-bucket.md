# Creating Federated Conformance Test GCS Buckets

This guide is aimed primarily at members of the Google GKE EngProd team for 
creating Google provided GCS buckets to be used by other providers for hosting
conformance results on TestGrid, but the general steps should be good practice
for anyone setting up a GCS bucket for automated uploads.

1) Use a separate dedicated [GCP project](https://cloud.google.com/storage/docs/projects), to further limit access to unrelated resources. We use [k8s-federated-conformance](http://console.cloud.google.com/home/dashboard?project=k8s-federated-conformance).

2) Create a new bucket in the GCP project. See the official [Creating Storage Buckets](https://cloud.google.com/storage/docs/creating-buckets) guide. Buckets should be used one to a provider. We use the naming scheme `k8s-conformance-$PROVIDER` eg `gs://k8s-conformance-openstack`.

3) Follow [Making Data Public](https://cloud.google.com/storage/docs/access-control/making-data-public) (specifically the "Making groups of objects publicly readable" section) to make the bucket readable by TestGrid.
  - This essentially involves adding `allUsers` to the bucket with `Storage Object Viewer` permission.

4) Create a matching service account, something like `$PROVIDER-logs` which will ultimately create an account like `openstack-logs@k8s-federated-conformance.iam.gserviceaccount.com`. See [Creating and Managing Service Accounts](https://cloud.google.com/iam/docs/creating-managing-service-accounts) for more details.

5) Add [`Storage Object Create`](https://cloud.google.com/storage/docs/access-control/iam-roles) permissions (`storage.objects.create`) to the service account created in 4). This allows the service account to create new entries. See also [Identity and Access Management](https://cloud.google.com/storage/docs/access-control/iam).

6) [Generate a service account credential](https://cloud.google.com/storage/docs/authentication#generating-a-private-key) file. Per the [gcloud auth activate-service-account](https://cloud.google.com/sdk/gcloud/reference/auth/activate-service-account) docs the JSON format is preferred. This file must be provided to the CI uploading the test results. It can be used with the `--key-file` flag in [`upload_e2e.py`](./upload_e2e.py).


