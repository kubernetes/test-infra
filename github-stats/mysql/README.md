Set-up Google Cloud SQL Proxy
-----------------------------

Create the database, from the console (recommended) or from command line:
```
gcloud sql instances create github-database
```

Fetch Service Account credentials and save it as "credential.json"
https://developers.google.com/identity/protocols/application-default-credentials

Create secret with the credentials:
```
kubectl create secret generic sqlproxy-credential-secret --from-file=credential.json
```

Create configmap to install ca-certificate and instances to listen to:
```
kubectl create configmap sqlproxy-config --from-literal=instances=${gcp_sql_project}:github-database=tcp:0.0.0.0:3306 --from-file=/etc/ssl/certs/ca-certificates.crt
```

Deployment
---------
As simple as:
```
kubectl apply -f sqlproxy.yaml
```
