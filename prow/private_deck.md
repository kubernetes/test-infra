# Setting up Private Deck

## 1) [User] Create a PR to Set up TenantIDs for prowjobs and Repos

Prow users should create a PR creating tenantID defaults for their org/repos and clusters. Once you set up a tenantID, all prowjobs labelled with that tenantID will only be visible on Deck instances created with the same tenantID. If you already have prowjobs that you don't want to lose access to on Deck, do this step last. If not, do it first to make sure prowjobs you want to keep sequestered do not appear on other instances of Deck.

The recommended way to add tenantIDs to prowjobs based on org/repo or cluster is through prowjob_default_entries in the prow config. This will apply the tenant ID to jobs with matching cluster AND repo. If you want to do cluster OR repo, create two entries in the config and use "*" for either field.
```yaml
prowjob_default_entries:
  - cluster: "build-private"
    repo: "*"
    config:
      tenant_id: 'private'
  - cluster: '*'
    repo: 'private'
    config:
      tenant_id: 'private'

```
This configuration is used both to apply tenantIDs to prowjobs, but is also used by Deck to filter out Tide information from orgRepos with tenantIDs that do not match. So even if you can lable all your prowjobs using cluster, make sure that all of your repos are given a tenantID as well.

Once the PR is created, Prow operators should review the PR to make sure that no other tenantIDs were affected by the change.

### Override TenantIDs

You can also define a tenantID for a given prowjob by defining it in the prowjob spec under spec.ProwJobDefault. This will override the tenantID assigned via prowjob defaults.

## 2) [Operator] Create a New Service Account and Bind it

```
kind: ServiceAccount
apiVersion: v1
metadata:
  namespace: default
  name: <SA_NAME>
  annotations:
    "iam.gke.io/gcp-service-account": "..."
```

``` bash
gcloud iam service-accounts add-iam-policy-binding \
  --project=PROJECT \
  --role=roles/iam.workloadIdentityUser \
  --member=serviceAccount:K8S_PROJECT.svc.id.goog[SOMEWHERE/SOMETHING] \
  SOMEBODY@PROJECT.iam.gserviceaccount.com

```

Once the service account is created, grant the service account Viewer access to the GCS bucket where test results are located.
## 3) [Operator] (Optional) Create a New OauthApp for Authentication

If you want to make the new Deck instance private, create a new oauth app using the Prowbot github account. 

You can follow this [Documentation](https://docs.github.com/en/developers/apps/building-oauth-apps/creating-an-oauth-app) to create the app: 
### Creating oauth Secrets

You will need to create two secrets populated with information from the oauth app

github-oauth-config:
```
data: 
  secret: {
    "client_id":"...",
    "client_secret":"...",
    "redirect_url":"...",
    "cookie_secret":"...",
    "final_redirect_url":"...",
    "scopes":[]
    }
```

oauth-config
```
data: 
  clientID: ...
  clientSecret: ...
  cookieSecret: ...
```

For more information on how to make these secrets take a look at the [Secrets Documentation](/prow/prow_secrets.md)

## 4 [User] Create the new Deck Deployment

When creating the new Deck Deployment, make sure to update the following fields:

- Update Service Account on new Deployment
- Update TenantID on new Deployment
    - Add `- --tenant-id=NEW_ID` under args in the Deck deployment spec
    - You can add this flag multiple times to allow multiple tenantIDs
- (Optional) Add a volume mount for the oauth app and update the oauth-config 
    - Here is an example of oauth2-proxy being used with github account validation. The oauth and oauth-config secrets are made in step 3.

    ```
    volumeMounts:
    ...
    - name: oauth2-proxy
        image: quay.io/oauth2-proxy/oauth2-proxy
        ports:
        - containerPort: 4180
          protocol: TCP
        args:
        - --provider=github
        - --github-org=ORG
        - --github-team=TEAM
        - --http-address=0.0.0.0:4180
        - --upstream=http://localhost:8080
        - --cookie-domain=DOMAIN
        - --cookie-name=COOKIE NAME (can be anything)
        - --cookie-samesite=none
        - --cookie-expire=23h
        - --email-domain=*
        livenessProbe:
          httpGet:
            path: /ping
            port: 4180
          initialDelaySeconds: 3
          periodSeconds: 3
        readinessProbe:
          httpGet:
            path: /ping
            port: 4180
          initialDelaySeconds: 3
          periodSeconds: 3
        env:
        - name: OAUTH2_PROXY_CLIENT_ID
          valueFrom:
            secretKeyRef:
              name: oauth
              key: clientID
        - name: OAUTH2_PROXY_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: oauth
              key: clientSecret
        - name: OAUTH2_PROXY_COOKIE_SECRET
          valueFrom:
            secretKeyRef:
              name: oauth
              key: cookieSecret
        - name: OAUTH2_PROXY_REDIRECT_URL
          value: https://prow.infra.cft.dev/oauth2/callback
    ```
    
    ```
    volumes:
    ...
    - name: oauth-config
        secret:
          secretName: oauth-config
    ```

[Here](https://github.com/GoogleCloudPlatform/oss-test-infra/blob/master/prow/oss/cluster/deck_blueprints_deployment.yaml) is an example private deployment.
## 5 [Operator] Use the New Deployment

In order to use the new Deployment you will need to:

1. Make a new static IP
  - On GCP go to VPC Networks -> ExternalIP
  - Click Reserve Static Address
  - Set the Region to Global
2. Create new Domain and configure DNS with new Static IP
3. Make a new Ingress with the new Domain
 - Create a new Managed Cert
 - Add the new Rule
 - Configure Ingress to use the managed cert
 - [Here](https://github.com/GoogleCloudPlatform/oss-test-infra/blob/e1f836416d1b3cd2cebc81454eb7f5f1febbc468/prow/oss/cluster/cluster.yaml#L128) is an example

 ## 6 [User] Update status check links
 
 In the prow config, add the new domain to target_urls and job_url_prefix_config like so:

 ```yaml
 target_urls:
    "*": https://oss-prow.knative.dev/tide
    "privateOrg/repo": https://DOMAIN/tide
```

 ```yaml
 job_url_prefix_config:
    "*": https://oss-prow.knative.dev/view/
    "privateOrg/repo": https://DOMAIN/view/
```

## 7 [Operator] Ensure that the public deck service account does not have access to the bucket for the jobs you wish to remain private
