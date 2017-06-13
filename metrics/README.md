# Bigquery metrics

This folder contains config files describing metrics that summarize data
in our Bigquery test result database. Config files are consumed
by the `metrics-bigquery` periodic prow job that runs the bigquery
image and scenario.  Each metric consists of a bigquery query and
a jq filter. The query is run every 24 hours to produce a json data
file containing the complete raw query results named with the format
'raw-yyyy-mm-dd.json'. The raw file is then filtered with the associated
jq filter and the results are stored in 'daily-yyyy-mm-dd.json'.  These
files are stored in the k8s-metrics GCS bucket in a directory named with
the metric name and persist for a year after their creation. Additionally,
the latest filtered results for a metric are stored in the root of the
k8s-metrics bucket and named with the format 'METRICNAME-latest.json'.

## Metrics

* failures - find jobs that have been failing the longest
    - [Config](configs/failures-config.yaml)
    - [failures-latest.json](http://storage.googleapis.com/k8s-metrics/failures-latest.json)
* flakes - find the flakiest jobs this week (and the flakiest tests in each job).
    - [Config](configs/flakes-config.yaml)
    - [flakes-latest.json](http://storage.googleapis.com/k8s-metrics/flakes-latest.json)
* job-flakes - compute consistency of all jobs
    - [Config](configs/job-flakes-config.yaml)
    - [job-flakes-latest.json](http://storage.googleapis.com/k8s-metrics/job-flakes-latest.json)
* weekly-consistency - compute overall weekly consistency for PRs
    - [Config](configs/weekly-consistency-config.yaml)
    - [weekly-consistency-latest.json](http://storage.googleapis.com/k8s-metrics/weekly-consistency-latest.json)

## Adding a new metric

To add a new metric, create a PR that adds a new yaml config file
specifying the metric name, the bigquery query to execute, and a
jq filter to filter the data for the daily and latest files. Find
the new metric on GCS 24 hours after merging.

## Consistency

Consistency means the test, job, pr always produced the same answer. For
example suppose we run a build of a job 5 times at the same commit:
* 5 passing runs, 0 failing runs: consistent
* 0 passing runs, 5 failing runs: consistent
* 1-4 passing runs, 1-4 failing runs: inconsistent aka flaked
