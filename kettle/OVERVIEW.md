# Kettle Workflow

## Overview
Kettle is a service tasked with tracking and uploading to [BigQuery] all completed jobs with results stored in the list of [Buckets]. This data can be then used to track metrics on failures, flakes, activity, etc.

This document is intended to walk maintainers through Kettle's workflow to get a better understanding of how to add features, fix bugs, or rearchitect the service. This document will be organized in a Top->Down way that will start at Kettle's ENTRYPOINT and work through each stage.

## ENTRYPOINT
Kettle's main process is the execution of `runner.sh` which:
- sets gcloud auth credentials
- creates initial "`bq config`"
- pulls most recent [Buckets]
- executes `update.py` on loof

`update.py` governs the flow of Kettle's three main stages:
- [make_db.py](#Make-Database): Collects every build from GCS in the given buckets and creates a database entry of results.
- [make_json.py+bq load](#Create-json-Results-and-Upload): Builds json representation of the database and uploads reults to the respective tables.
- [stream.py](#Stream-Results): Wait for pub-sub events for completed builds and upload as results surface.

# Make Database
Flags:
- --buckets (str): Path to YAML that defines all the gcs buckets to collect jobs from
- --junit (bool): If true, collect Junit xml test results
- --threads (int): Number of threads to run concurrently with
- --buildlimit (int): **Used in staging*  colect only N builds on each job

`make_db.py` does the work of determine all the builds to collect and store to the database. It aggrigates all the builds of two flavors: `pr` and `non-pr` builds. It searches gcs for build paths or generates build paths if they are "incremental builds" (monotomically increasing). It passes the work of collecting build information and results to threads that collect information. It then does a best-effort attempt to insert the build results to the DB, commiting the instert every 200 builds.

# Create JSON Results and Upload
This stage gets run for each [BugQuery] table that Kettle is tasked with uploading data to. Typically looking like either:
- Fixed Time: `pypy3 make_json.py --days <num> | pv | gzip > build_<table>.json.gz`
    and `bq load --source_format=NEWLINE_DELIMITED_JSON --max_bad_records={MAX_BAD_RECORDS} k8s-gubernator:build.<table> build_<table>.json.gz schema.json`
- All Results: `pypy3 make_json.py | pv | gzip > build_<table>.json.gz`
    and `bq load --source_format=NEWLINE_DELIMITED_JSON --max_bad_records={MAX_BAD_RECORDS} k8s-gubernator:build.<table> build_<table>.json.gz schema.json`

### Make Json
`make_json.py` prepares an incremental table to track builds it has emmited to BQ. This table is named `build_emitted_<days>` (if days flag passed) or `build_emitted` otherwise. *This is important because if you change the days AND NOT the table being uploaded to, you will get duplicate results. If the `--reset_emmited` flag is passed, it will refresh the incremental table for fresh data. It then walks all of the builds to fetch within `<days>` or since epoch if unset, and dumps each as a json object to a build `tar.gz`.

### BQ Load
This step uploads all of the `tar.gz` data to BQ while conforming to the [Schema], this schema must match the defined fields within [BigQuery] (see README for detials on adding fields).

# Stream Results
After all historical data has been uploaded, Kettle enters a Streaming phase. It subscribes to pub-sub results from `kubernetes-jenkins/gcs-changes/kettle` (or the specified GCS subscription path) and listens for events (Jobs completing). When a job triggers an event, it:
- will collect data for this job
- insert it in the database
- create a BQ client
- gets the builds it just injected
- serialized the rows to json
- inserts it into the tables (from flag)
- adds the data to the respective incremental tables

[BigQuery]: https://console.cloud.google.com/bigquery?utm_source=bqui&utm_medium=link&utm_campaign=classic&project=k8s-gubernator
[Buckets]: https://github.com/kubernetes/test-infra/blob/master/kettle/buckets.yaml
[Schema]: https://github.com/kubernetes/test-infra/blob/master/kettle/schema.json