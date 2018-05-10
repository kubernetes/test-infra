# `entrypoint`

`entrypoint` wraps a process and records its output to `stdout` and `stderr` as well as its exit
code, recording both to disk. The utility will exit with a non-zero exit code if the wrapped
process fails or if the utility has a fatal error.

This utility is intended to be used with [`sidecar`](./../sidecar/README.md), which will
watch the files written by this utility and report on the status of the wrapped process.

`entrypoint` can be configured by either passing in flags or by specifying a full set of options
as JSON in the `$ENTRYPOINT_OPTIONS` environment variable, which has the form:

```json
{
    "args": [
        "/bin/ls",
        "-la"
    ],
    "timeout": 7200000000000,
    "grace_period": 15000000000,
    "process_log": "/logs/process-log.txt",
    "marker_file": "/logs/marker-file.txt"
}
```

Note: the `"timeout"` and `"grace_period"` fields hold the duration in nanoseconds.