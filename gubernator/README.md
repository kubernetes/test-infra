# [Gubernator](//gubernator.k8s.io/)

Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

https://gubernator.k8s.io/

# Adding a repository to the PR Dashboard

To make Gubernator's [PR Dashboard](https://gubernator.k8s.io/pr) work
on another repository, it needs to receive webhook events.

Go to Settings -> Webhooks on the repository (or organization) you want to add.

Add a new webhook with these options:

Payload URL: https://github-dot-k8s-gubernator.appspot.com/webhook
Secret: Ask test-infra oncall.
Select: "Send me everything"

Gubernator will use the events it receives to build information about PRs, so
only updates after the webhook is added will be shown on the dashboard.

# Development

- Install the Google Cloud SDK: https://cloud.google.com/sdk/
- Run locally using `dev_appserver.py app.yaml` and visit http://localhost:8080
- Test and lint using `./test-gubernator.sh`
- Deploy with `make deploy` followed by `make migrate`

# Deployment

- Visit /config on the new deployment to configure GitHub [OAuth logins](https://github.com/settings/applications)
  and webhook secrets.

# GCS Layout

In order to correctly interpret jobs results, in GCS, Gubernator expects that
any one job directory is laid out in a specific manner, and that job directories
are laid out in a specific manner relative to each other.

## Job Artifact GCS Layout

Every run should upload `started.json`, `finished.json`, and `build-log.txt`, and
can optionally upload JUnit XML and/or other files to the `artifacts/` directory.
For a single build of a job, Gubernator expects the following layout in GCS:

```
.
├── artifacts         # all artifacts must be placed under this directory
│   └── junit_00.xml  # JUnit XML reports from the build
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

Any artifacts from the build should be placed under `./artifacts/`. Any JUnit
XML reports should be named `junit_*.xml` and placed under `./artifacts` as well.

## Test Properties [Optional]

Test properties are a set of key value pairs defined on the test case and are optional. The test 
result file `junit_*.xml` contains a list of test cases and the properties associated with it.
These properties can be later parsed by any aggregator like testgrid, and used to collect metrics 
about the test case.

The properties can be defined as:

```xml
<testcase ...>
  <properties>
    <property>
        <name>key1</name>
        <value>value1</value>
    </property>
    <property>
        <name>key2</name>
        <value>value2</value>
    </property>
  </properties>
</testcase>
```

## GCS Bucket Layout

In your bucket, a number of directories are required to store the output of all
the different types of jobs:

```
.
├── logs                    # periodic and postsubmit jobs live here
│   └── other_job_name      # contains all the builds of a job
│      ├── build_number     # contains job artifacts, as above
│      └── latest-build.txt # contains the latest build id of a job
└── pr-logs
    ├── directory                # symlinks for builds live here
    │   └── job_name             # contains all symlinks for a job
    │       ├── build_number.txt # contains one line: location of artifacts for the build
    │       └── latest-build.txt # contains the latest build id of a job
    └── pull
        ├── batch                # batch jobs live here
        │   └── job_name         # contains all the builds of a job
        │       └── build_number # contains job artifacts, as above
        └── org_repo                     # jobs testing PRs for org/repo live here
            └── pull_number              # jobs running for a PR with pull_number live here
                └── job_name             # all builds for the job for this pr live here
                    └── build_number     # contains job artifacts, as above
                    └── latest-build.txt # contains the latest build id of a job
```

# Migrations

1. 2018-01-09: GitHub webhook and oauth secrets must be reconfigured. Visit
   /config on your deployment to enter the new values.
