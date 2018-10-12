# `clonerefs`

`clonerefs` clones code under test at the specified locations. Regardless of the success or failure
of clone operations, this utility will have an exit code of `0` and will record the clone operation
status to the specified log file. Clone records have the form:

```json
[
    {
        "failed": false,
        "refs": {
            "org": "kubernetes",
            "repo": "kubernetes",
            "base_ref": "master",
            "base_sha": "a36820b10cde020818b8dd437e285d0e2e7d5e98",
            "pulls": [
                {
                    "number": 123,
                    "author": "smarterclayton",
                    "sha": "2b58234a8aee0d55918b158a3b38c292d6a95ef7"
                }
            ]
        },
        "commands": [
            {
                "command": "git init",
                "output": "Reinitialized existing Git repository in /go/src/k8s.io/kubernetes/.git/",
                "error": ""
            }
        ]
    }
]
```

Note: the utility _will_ exit with a non-zero status if a fatal error is detected and no clone
operations can even begin to run.

This utility is intended to be used with [`initupload`](./../initupload/README.md), which will
decode the JSON output by `clonerefs` and can format it for human consumption.

`clonerefs` can be configured by either passing in flags or by specifying a full set of options
as JSON in the `$CLONEREFS_OPTIONS` environment variable, which has the form:

```json
{
    "src_root": "/go",
    "log": "/logs/clone-log.txt",
    "git_user_name": "ci-robot",
    "git_user_email": "ci-robot@k8s.io",
    "refs": [
        {
            "org": "kubernetes",
            "repo": "kubernetes",
            "base_ref": "master",
            "base_sha": "a36820b10cde020818b8dd437e285d0e2e7d5e98",
            "pulls": [
                {
                    "number": 123,
                    "author": "smarterclayton",
                    "sha": "2b58234a8aee0d55918b158a3b38c292d6a95ef7"
                }
            ],
            "skip_submodules": true
        }
    ]
}
```