Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

https://k8s-gubernator.appspot.com/

For development:

- Install the Google Cloud SDK: https://cloud.google.com/sdk/
- Run locally using `dev_appserver.py` and visit http://localhost:8080
- Test and lint using `./test-gubernator.sh`
- Deploy with `make deploy` followed by `make migrate`

For deployment:

- Get the "Gubernator Github Webhook Secret" (ask test-infra for help) and write
  it to `github/webhook_secret`.
- Set up `secrets.json` to support Github [OAuth logins](https://github.com/settings/applications).
  The skeleton might look like:

```json
    {
        "k8s-gubernator.appspot.com": {
            "session": "(128+  bits of entropy for signing secure cookies)",
            "github_client": {
                "id": "(client_id for the oauth application)",
                "secret": "(client_secret for the oauth application)"
            }
        }
    }
```

# GCS Layout

In order to correctly interpret jobs results, in GCS, Gubernator expects that
any one job directory is laid out in a specific manner, and that job directories
are laid out in a specific manner relative to each other.

## Job Artifact GCS Layout

Every run should upload `started.json`, `finished.json`, and `build-log.txt`, and
can optionally upload jUnit XML and/or other files to the `artifacts/` directory.
For a single build of a job, Gubernator expects the following layout in GCS:

```
.
├── artifacts         # all artifacts must be placed under this directory
│   └── junit_00.xml  # jUnit XML reports from the build
├── build-log.txt     # std{out,err} from the build
├── finished.json     # metadata uploaded once the build finishes
└── started.json      # metadata uploaded once the build starts
```

The following fields in `started.json` are honored:

```json
{
    "timestamp": "seconds after UNIX epoch that the build started",
    "pull": "$PULL_REFS from the run",
    "repos": {
        "org/repo": "git version of the repo used in the test",
    },
}
```

The following fields in `finished.json` are honored:

```json
{
    "timestamp": "seconds after UNIX epoch that the build finished",
    "result": "SUCCESS or FAILURE, the result of the build",
    "metadata": "dictionary of additional key-value pairs that will be displayed to the user",
}
```

Any artifacts from the build should be placed under `./artifacts/`. Any jUnit
XML reports should be named `junit_*.xml` and placed under `./artifacts` as well.

## GCS Bucket Layout

In your bucket, a number of directories are required to store the output of all
the different types of jobs:

```
.
├── logs                 # periodic and postsubmit jobs live here
│   └── other_job_name   # contains all the builds of a job
│       └── build_number # contains job artifacts, as above
└── pr-logs
    ├── directory                # symlinks for builds live here
    │   └── job_name             # contains all symlinks for a job
    │       └── build_number.txt # contains one line: location of artifacts for the build
    └── pull
        ├── batch                # batch jobs live here
        │   └── job_name         # contains all the builds of a job
        │       └── build_number # contains job artifacts, as above
        └── org_repo                 # jobs testing PRs for org/repo live here
            └── pull_number          # jobs running for a PR with pull_number live here
                └── job_name         # all builds for the job for this pr live here
                    └── build_number # contains job artifacts, as above
```