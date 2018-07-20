KETTLE -- Kubernetes Extract Tests/Transform/Load Engine
======

This collects test results scattered across a variety of GCS buckets,
stores them in a local SQLite database, and outputs newline-delimited JSON files
for import into BigQuery.

Results are stored in the [k8s-gubernator:build BigQuery dataset](https://bigquery.cloud.google.com/dataset/k8s-gubernator:build),
which is publicly accessible.

Running
=======
Use `pip install -r requirements.txt` to install dependencies.

Deploying
=======
Kettle runs as a pod in Gubernator's GKE cluster.

buckets.yaml is automatically fetched from GitHub.

When code changes, run 'make push update' to deploy a new version. If it fails,
use "make rollback" to revert to the previous deploy.

If deployment.yaml changes, run 'make push deploy'.

Troubleshooting
==============
Occasionally we run into a situation where there's no data in the Bigquery metrics dashboard in Velodrome. If you run into this, you can restart the Kettle job by following the steps [here](/troubleshoot.md)
