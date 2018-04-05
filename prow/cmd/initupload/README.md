# `initupload`

`initupload` reads clone records placed by `clonerefs` in order to determine job status. The status
and logs from the clone operations are uploaded to cloud storage at a path resolved from the job
configuration. This utility will exit with a non-zero exit code if the clone records indicate that
any clone operations failed, as well as if any fatal errors are encountered in this utility itself.

`initupload` can be configured by either passing in flags or by specifying a full set of options
as JSON in the `$INITUPLOAD_OPTIONS` environment variable, which has the same form as that for
`gcsupload`, plus the `"log"` field. See [that documentation](./../gcsupload/README.md) for
an explanation.

```json
{
    "log": "/logs/clone-log.txt",
    "bucket": "kubernetes-jenkins",
    "sub_dir": "",
    "items": [
        "/logs/artifacts/"
    ],
    "path_strategy": "legacy",
    "default_org": "kubernetes",
    "default_repo": "kubernetes",
    "gcs_credentials_file": "/secrets/gcs/service-account.json",
    "dry_run": "false"
}
```

In addition to this configuration for the tool, the `$JOB_SPEC` environment variable should be
present to provide the contents of the Prow downward API for jobs. This data is used to resolve
the exact location in GCS to which artifacts and logs will be pushed.
