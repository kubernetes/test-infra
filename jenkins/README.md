# How to work with Jenkins jobs

Each job has:

1. A Jenkins job entry, in either [kubernetes-jenkins](job-configs/kubernetes-jenkins) or [kubernetes-jenkins-pull](job-configs/kubernetes-jenkins-pull).

1. A json entry in [job configs](../jobs/config.json)

1. (Required for e2e jobs) a `foo.env` match job `foo` in [/jobs](../jobs)

1. (Required for ci jobs) a [testgrid config entry](../testgrid/config/config.yaml)

Ping @kubernetes/test-infra-maintainers if you have any questions.

# Add a Jenkins job

Say you want to add a new job, foo.

1. For e2e ci job, add an entry to [`test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci.yaml`](jenkins/job-configs/kubernetes-jenkins/bootstrap-ci.yaml).
   
   For PR job, add an entry to [`test-infra/jenkins/job-configs/kubernetes-jenkins-pull/bootstrap-pull.yaml`](jenkins/job-configs/kubernetes-jenkins-pull/bootstrap-pull.yaml)

   For build job, add an entry to [`test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml`](jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml)

   For soak job, add an entry to [`test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml`](jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml)

   For maintenance job, add an entry to [`test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml`](jenkins/job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml)

   [bootstrap flags](bootstrap.py#L806-L838) will help you determine which flag you might need.


1. Add an entry to [config.json](../jobs/config.json). Choose an appropriate [scenario](../scenarios) file and args. 

1. If it's an e2e job, add foo.env to [jobs](../jobs), which defines environment variables your job will be using. You can reference it from other e2e jobs.

1. Add your new job to [`test-infra/testgrid/config/config.yaml`](../testgrid/config/config.yaml), instruction can be found in [here](../testgrid/config/README.md).

1. Make sure all presubmit tests pass. Running [`test-infra/jenkins/bootstrap_test.py`](jenkins/bootstrap_test.py) and `bazel test //...` locally is an quick way to trigger most of the unit tests.

# Run a Jenkins job locally

Below paths are from the root of the `kubernetes/test-infra` repo.

The straight forward way to mimic a job run on Jenkins is, for example:
```
./jenkins/bootstrap.py --job=ci-kubernetes-e2e-gce-canary --json=1 --bare --timeout=40
```

If you want to upload the log and display in gubernator, you can append
```
--service-account=/path/to/service-account-cred.json --upload='gs://your-bucket-name/logs'
```

Reference: [Creating and Enabling Service Accounts for Instances](https://cloud.google.com/compute/docs/access/create-enable-service-accounts-for-instances)

You can also specify `--repo`, `--branch`, and other bootstrap flags, details can be found by running `./jenkins/bootstrap.py --help`

If you want to overwrite some scenario flags, you can change [config.json](../jobs/config.json) locally.

Alternatively, if you do not care about logs and artifacts, you can also run:
```
export BOOTSTRAP_MIGRATION=true # https://github.com/kubernetes/test-infra/pull/1801#discussion_r102347949
export BUILD_NUMBER=SOMETHING
export JOB_NUMBER=JOB_NAME
./scenario/your_scenario.py --env-file=your_env_file --scenario-flag=your-flag
```

