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

