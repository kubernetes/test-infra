# Prow Secrets Management

Secrets in prow service/build clusters are managed with Kubernetes External
Secrets, which is responsible for one-way syncing secret values from major
secret manager providers such as GCP, Azure, and AWS secret managers into
kubernetes clusters, based on `ExternalSecret` custom resource defined in
cluster (As shown in example below).

_Note: the instructions below are only for GCP secret manager, for
authenticating with other providers please refer to
https://github.com/external-secrets/kubernetes-external-secrets#backends_

## Set Up (Prow maintainers)

This is performed by prow service/build clusters maintainer.

1. Create a GKE cluster and enable workload identity by
   following [`workload-identity`](/workload-identity/README.md).
1. Deploy `kubernetes-external-secrets_crd.yaml`,
   `kubernetes-external-secrets_deployment.yaml`,
   `kubernetes-external-secrets_rbac.yaml`,
   and  `kubernetes-external-secrets_service.yaml` under
   [`config/prow/cluster`](/config/prow/cluster). The deployment file assumes
   using the same service account name as used in step #1

## Usage (Prow clients)

This is performed by prow serving/build cluster clients.

1. In the GCP project that stores secrets with google secret manager, grant the
   `roles/secretmanager.viewer` and `roles/secretmanager.secretAccessor`
   permission to the GCP service account used above
1. Create secret in google secret manager, assume it's named `my-precious-secret`
1. Create kubernetes external secrets custom resource by:
   ```
   apiVersion: kubernetes-client.io/v1
   kind: ExternalSecret
   metadata:
     name: <my-precious-secret-kes-name>    # name of the k8s external secret and the k8s secret
     namespace:  <ns-where-secret-is-used>
   spec:
     backendType: gcpSecretsManager
     projectId: <my-gsm-secret-project>
     data:
     - key: <my-gsm-secret-name>     # name of the GCP secret
       name: <my-kubernetes-secret-name>   # key name in the k8s secret
       version: latest    # version of the GCP secret
       # Property to extract if secret in backend is a JSON object,
       # remove this line if using the GCP secret value straight
       property: value
   ```

Within 10 seconds, a secret will be created automatically:
```
apiVersion: v1
kind: Secret
metadata:
  name: <my-precious-secret-kes-name>
  namespace:  <ns-where-secret-is-used>
data:
  <my-kubernetes-secret-name>: <value_read_from_gsm>
```
