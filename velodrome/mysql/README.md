Overview
========

This deployment is used to set-up the Google Cloud SQL Proxy. The easiest way to
access a Cloud SQL database is to use the proxy so that you connect to it, and
it will redirect everything to the actual Cloud SQL database. This is secure and
simple and uses the Service Account key to connect.

You can get more info about the SQL database schema here: [../sql](../sql/)

Set-up Google Cloud SQL Proxy
=============================

Create the database, from the console (recommended) or from command line:
```
gcloud sql instances create github-database
```

Create a new service account (with Editor role) and fetch its credentials, save
it as "credential.json"
https://cloud.google.com/storage/docs/authentication#service_accounts

Create secret with the credentials:
```
kubectl create secret generic service-account-token --from-file=credential.json
```

Create secret for instances to listen to:
```
kubectl create secret generic sqlproxy --from-literal=instances=${gcp_sql_project}:${zone}:github-database=tcp:0.0.0.0:3306
```
