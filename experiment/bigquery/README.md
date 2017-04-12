# Bigquery scripts

This folder contains scripts to summarize data in our Bigquery test result
database.

## Assumptions

We assume your machine has jq and bq. The bq program is part of gcloud.

So please `apt-get install jq google-cloud-sdk` (see [gcloud install
instructions](https://cloud.google.com/sdk/downloads#apt-get)).

## Consistency

Consistency means the test, job, pr always produced the same answer. For
example suppose we run a build of a job 5 times at the same commit:
* 5 passing runs, 0 failing runs: consistent
* 0 passing runs, 5 failing runs: consistent
* 1-4 passing runs, 1-4 failing runs: inconsistent aka flaked

## Scripts

* `flakes.sh` - find the flakiest jobs this week (and the flakiest tests in each job).
    - Uses `flakes.sql` to extract and group data from BigQuery
    - Usage: `flakes.sh | tee flakes-$(date %Y-%m-%d).json`
    - Latest results: [flakes-latest.json](flakes-latest.json)
* `job-flakes.sh` - compute consistency of all jobs
    - Uses `job-flakes.sql` to compute this data
    - Usage: `job-flakes.sh | tee job-flakes-$(date %Y-%m-%d).json`
    - Latest results: [job-flakes-latest.json](job-flakes-latest.json)
* `weekly-consistency.sh` - compute overall weekly consistency for PRs
    - Uses `weekly-consistency.sql` to compute this data
    - Usage: `weekly-consistency.sh | tee weekly-consistency.json`
    - Latest results: [weekly-consistency.json](weekly-consistency.json)


Future PRs will migrate other queries from [this spreadsheet](https://docs.google.com/spreadsheets/d/16nQPj_40xBgPLprj1DkKVTQ-BgQzO4A707q_JPsdxzY/edit).
