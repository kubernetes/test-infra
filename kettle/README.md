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
