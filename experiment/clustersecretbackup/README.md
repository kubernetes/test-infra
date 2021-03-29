# clustersecretbackup

Cluster secret backup is a tool backing up secrets in a cluster in Google Secret
Manager.

## Prerequisite

- Set `GOOGLE_APPLICATION_CREDENTIALS`
- Already authenticated with cluster to be backed up

## Usage

This tool can be invoked locally, by:

- `--project`: The GCP project that secrets will be backed up in.
- `--cluster-context`: The cluster context name that need to be backed up, must be full form such as <PROVIDER>_<PROJECT>_<ZONE>_<CLUSTER>.
- `--namespace`: The namespace(s) to be backed up, can be passed in repeatedly.
- `--update`: Controls whether update existing secret or not, if false then
  secret will not be updated when the secret already exist in gsm.
- `--dryrun`: Controls whether write to Google secret manager or not.
