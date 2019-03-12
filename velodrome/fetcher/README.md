Overview
========

Fetcher retrieves a github repository history and stores it in a MySQL local
database.

For now, it downloads three types of resources:
- Issues (including pull-requests)
- Events (associated to issues)
- Comments (regular comments and review comments)

All of these resources will allow us to:
- Compute average time-to-resolution for an issue/pull-request
- Compute time between label creation/removal: lgtm'd, merged
- break-down based on specific flags (size, priority, ...)

The model is described more precisely in [../sql](../sql/).

Fetcher only downloads what is not already in the SQL database by trying to find
the latest events it knows about. It will poll from GitHub on a regular basis,
and this can be configured with the `--frequency` flag.

There is no set-up required as `fetcher` will create the github database if it
doesn't exist, along with the various required tables.

Testing locally
===============

It is strongly suggested NOT to download Kubernetes/kubernetes from scratch with
the fetcher, as it puts a lot of pressure on the token and takes a significant
amount of time. It has been done already and one should use an export of the
existing database.

It is fine to download a smaller repository for testing, like
kubernetes/test-infra or kubernetes/contrib.

The SQL database can be set-up on Google Cloud SQL, and then accessed with the
cloud-sql proxy as described here:
https://github.com/GoogleCloudPlatform/cloudsql-proxy


Create a new version
====================
Push
----

Run the push script:
```
make push IMG=gcr.io/your-repo/fetcher
```

Update deployment
-----------------

Update the `deployment.yaml` with the new generated version.
