# Prow Secrets Management

Secrets in prow service/build clusters are managed with [Kubernetes External
Secrets](https://github.com/external-secrets/kubernetes-external-secrets), which is responsible for one-way syncing secret values from major
secret manager providers such as GCP, Azure, and AWS secret managers into
kubernetes clusters, based on `ExternalSecret` custom resource defined in
cluster (As shown in example below).

_Note: the instructions below are only for GCP secret manager, for
authenticating with other providers please refer to
https://github.com/external-secrets/kubernetes-external-secrets#backends_

## Set Up (Prow maintainers)

This is performed by prow service/build clusters maintainer.

1. In the cluster that the secrets are synced to, enable workload identity by
   following [`workload-identity`](/workload-identity/README.md).
1. Deploy `kubernetes-external-secrets_crd.yaml`,
   `kubernetes-external-secrets_deployment.yaml`,
   `kubernetes-external-secrets_rbac.yaml`,
   and  `kubernetes-external-secrets_service.yaml` under
   [`config/prow/cluster`](/config/prow/cluster). The deployment file assumes
   using the same service account name as used in step #1

TODO(chaodaiG): recommend use of postsubmit deploy job for managing the
deployment once this PR is merged.

## Usage (Prow clients)

This is performed by prow serving/build cluster clients. Note that the GCP
project mentioned here doesn't have to, and normally is not the same GCP project
where the prow service/build clusters are located.

1. In the GCP project that stores secrets with google secret manager, grant the
   `roles/secretmanager.viewer` and `roles/secretmanager.secretAccessor`
   permission to the GCP service account used above, by running:
   ```
   gcloud beta secrets add-iam-policy-binding <my-gsm-secret-name> --member="serviceAccount:<same-service-account-for-workload-identity>" --role=<role> --project=<my-gsm-secret-project>
   ```
   The above command ensures that the service account used by prow can only
   access the secret name `<my-gsm-secret-name>` in the GCP project owned by
   clients. The service account used for prow.k8s.io is defined in
   [`trusted_serviceaccounts.yaml`](https://github.com/kubernetes/test-infra/blob/1b2153ebe2809727a45c5b930647b2a3609dd7e7/config/prow/cluster/trusted_serviceaccounts.yaml#L46)

2. Create secret in google secret manager
3. Create kubernetes external secrets custom resource by:
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

Within 10 seconds (determined by `POLLER_INTERVAL_MILLISECONDS` envvar on deployment), a secret will be created automatically:
```
apiVersion: v1
kind: Secret
metadata:
  name: <my-precious-secret-kes-name>
  namespace:  <ns-where-secret-is-used>
data:
  <my-kubernetes-secret-name>: <value_read_from_gsm>
```

The `Secret` will be updated automatically when the secret value in gsm changed
or the `ExternalSecret` is changed. The secret will not be deleted by kubernetes
external secret, even in the case of `ExternalSecret` CR is deleted from the
cluster. Deletion of `Secret` will need to be handled separately if desired.
