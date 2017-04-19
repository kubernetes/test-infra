# Bigquery metrics

This folder contains config files describing metrics that summarize data in our Bigquery test result
database. The config files are consumed by the metrics-bigquery periodic prow job that runs the bigquery image and scenario. Each metric consists of a bigquery query and a jq filter.  The query is run every 24 hours to produce a json data file containing the complete query results named with the format METRICNAME-yyyy-mm-dd.json. These files persist for a year after their creation. Additionally, every time a query completes the associated jq filter is applied to the complete query results and the output is stored in the METRICNAME-latest.json file.

## Metrics

* failures - find jobs that have been failing the longest
    - Config: [failures-config.yaml](failures-config.yaml)
    - [Latest results](http://storage.googleapis.com/k8s-metrics/failures/failures-latest.json)
* flakes - find the flakiest jobs this week (and the flakiest tests in each job).
    - Config: [flakes-config.yaml](flakes-config.yaml)
    - [Latest results](http://storage.googleapis.com/k8s-metrics/flakes/flakes-latest.json)
* job-flakes - compute consistency of all jobs
    - Config: [job-flakes-config.yaml](job-flakes-config.yaml)
    - [Latest results](http://storage.googleapis.com/k8s-metrics/job-flakes/job-flakes-latest.json)
* weekly-consistency - compute overall weekly consistency for PRs
    - Config: [weekly-consistency-config.yaml](weekly-consistency-config.yaml)
    - [Latest results](http://storage.googleapis.com/k8s-metrics/weekly-consistency/weekly-consistency-latest.json)

## Adding a new metric

To add a new metric, create a PR that adds a new yaml config file specifying the metric name, the bigquery query to execute, and a jq filter to filter the data for the "METRICNAME-latest.json" file.
Then add the path to the config file as a --config flag value under the args key of the metrics-bigquery job in [test-infra/jobs/config.json](../jobs/config.json). The new metric should have data available on GCS within 24 hours of the changes being merged into the test-infra repo.

## Consistency

Consistency means the test, job, pr always produced the same answer. For
example suppose we run a build of a job 5 times at the same commit:
* 5 passing runs, 0 failing runs: consistent
* 0 passing runs, 5 failing runs: consistent
* 1-4 passing runs, 1-4 failing runs: inconsistent aka flaked


Future PRs will migrate other queries from [this spreadsheet](https://docs.google.com/spreadsheets/d/16nQPj_40xBgPLprj1DkKVTQ-BgQzO4A707q_JPsdxzY/edit).
