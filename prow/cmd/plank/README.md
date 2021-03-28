# Plank

Plank is the controller that manages the job execution and lifecycle for jobs running in k8s.

### Usage
```bash
$ bazel run //prow/cmd/plank -- --help
```

### Configuration
GCS and S3 are supported as the job log storage.

```yaml
# config.yaml

plank:
  # used to link to job results for decorated jobs (with pod utilities)
  job_url_prefix_config:
    '*': https://<domain>/view
  # used to link to job results for non decorated jobs (without pod utilities)
  job_url_template: 'https://<domain>/view/<bucket-name>/pr-logs/pull/{{.Spec.Refs.Repo}}/{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}/{{.Spec.Job}}/{{.Status.BuildID}}'
  report_template: '[Full PR test history](https://<domain>/pr-history?org={{.Spec.Refs.Org}}&repo={{.Spec.Refs.Repo}}&pr={{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}})'
  default_decoration_configs:
  # All entries that match a job are used, later entries override previous values.
  # Omission of 'repo' and 'cluster' fields makes this entry match all jobs.
  - config:
      timeout: 4h
      grace_period: 15s
      utility_images: # pull specs for container images used to construct job pods
        clonerefs: gcr.io/k8s-prow/clonerefs:v20190221-d14461a
        initupload: gcr.io/k8s-prow/initupload:v20190221-d14461a
        entrypoint: gcr.io/k8s-prow/entrypoint:v20190221-d14461a
        sidecar: gcr.io/k8s-prow/sidecar:v20190221-d14461a
      gcs_configuration: # configuration for uploading job results to GCS
        bucket: <bucket-name> or s3://<bucket-name>
        path_strategy: explicit # or `legacy`, `single`
        default_org: <github-org> # should not need this if `strategy` is set to explicit
        default_repo: <github-repo> # should not need this if `strategy` is set to explicit
      gcs_credentials_secret: <secret-name> # the name of the secret that stores cloud provider credentials
      ssh_key_secrets:
        - ssh-secret # name of the secret that stores the bot's ssh keys for GitHub, doesn't matter what the key of the map is and it will just uses the values
  - repo: "^org/" # some regexp to match against <org/repo>
    config:
      timeout:2h
  - cluster: "-trusted$" #some regexp to match against the cluster name
    config:
      # example override to use k8s SA with GCP workload identity rather than
      # a GCP service account key file.
      gcs_credentials_secret: ""
      default_service_account_name: <k8s-sa-name>
```

