# Bigquery scripts

This folder contains scripts to summarize data in our Bigquery test result
database.

## Assumptions

We assume your machine has jq and bq. The bq program is part of gcloud.

So please `apt-get install jq google-cloud-sdk` (see [gcloud install
instructions](https://cloud.google.com/sdk/downloads#apt-get)).

## Scripts

* `flakes.sh` - find the flakiest jobs this week (and the flakiest tests in each
  job).
    - Uses `flakes.sql` to extract and group data from BigQuery
    - Usage `flakes.sh | tee flakes-$(date %Y-%m-%d).json`
    - Latest results: [flakes-latest](https://github.com/kubernetes/test-infra/blob/master/experiment/bigquery/flakes-latest.json)


Future PRs will migrate other queries from [this spreadsheet](https://docs.google.com/spreadsheets/d/16nQPj_40xBgPLprj1DkKVTQ-BgQzO4A707q_JPsdxzY/edit).
