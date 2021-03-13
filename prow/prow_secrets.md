# Prow Secrets Management

Prow secrets are managed with Kubernetes External Secrets

## Set Up (Prow maintainers)

This is performed by prow service/build clusters maintainer.

1. Create a GKE cluster and enable workload identity by
   following [`workload-identity`](/workload-identity/README.md), using
   `kubernetes-external-secrets-sa` as service account name, and
   `kubernetes-external-secrets-sa@@k8s-prow.iam.gserviceaccount.com` service
   account created in `k8s-prow` GCP project
1. Deploy `kubernetes-external-secrets_crd.yaml`,
   `kubernetes-external-secrets_deployment.yaml`,
   `kubernetes-external-secrets_rbac.yaml`,
   and  `kubernetes-external-secrets_service.yaml` under
   [`config/prow/cluster`](/config/prow/cluster). The deployment file assumes
   using the same service account name as in step #1

## Usage (Prow clients)

This is performed by prow clients, presumably teams managing their own build cluster(s).

1. In the GCP project that stores secrets with google secret manager, grant the
   `roles/secretmanager.viewer` permission to the GCP service account used in
   step #1
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
