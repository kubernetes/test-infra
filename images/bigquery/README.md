# bigquery

The `gcr.io/k8s-testimages/bigquery` image is used to run [`/metrics/bigquery.py`] and [`/kettle/monitor.py`]

It is mostly present to ensure the following is available:
- `python` - required by `gcloud` and `bq`
- `python3` - required by `/metrics/bigquery.py`
- `jq` - invoked by the script to transform json results
- `bq` - invoked by the script to hit bigquery (comes with `gcloud`)
- python libraries used by the script

[`/metrics/bigquery.py`]: /metrics
[`/kettle/monitor.py`]: /kettle/monitor.py
