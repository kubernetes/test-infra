# `sidecar`

`sidecar` watches disk for files containing a the `std{out,err}` output from a process as well as
its exit code; when the exit code has been written, this utility uploads a status object, the logs
from the process and any other specified artifacts to cloud storage. The utility will exit with the
exit code of the wrapped process or otherwise non-zero if the utility has a fatal error.

This utility is intended to be used with [`entrypoint`](./../entrypoint/README.md), which will
write the files watched by this utility.

`sidecar` can be configured by either passing in flags or by specifying a full set of options
as JSON in the `$SIDECAR_OPTIONS` environment variable, which has the same form as that for
`gcsupload`, plus the `"process_log"` and `"marker_file"` fields. See
[that documentation](./../gcsupload/README.md) for an explanation.

```json
{
    "wrapper_options": {
        "process_log": "/logs/process-log.txt",
        "marker_file": "/logs/marker-file.txt"
    },
    "gcs_options": {
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
}
```

In addition to this configuration for the tool, the `$JOB_SPEC` environment variable should be
present to provide the contents of the Prow downward API for jobs. This data is used to resolve
the exact location in GCS to which artifacts and logs will be pushed.
