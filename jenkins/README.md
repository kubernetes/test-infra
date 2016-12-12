# How to work with Jenkins jobs

All the job scripts are in the [test-infra/jobs/{job-name}.sh](https://github.com/kubernetes/test-infra/tree/master/jobs) files now.

# Add a Jenkins job

Say you want to add a new job, foo.

1. For e2e ci job, add an entry to [test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci.yaml](https://github.com/kubernetes/test-infra/blob/master/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci.yaml).
   
   For pr job, add an entry to [test-infra/jenkins/job-configs/kubernetes-jenkins-pull/bootstrap-pull.yaml](https://github.com/kubernetes/test-infra/blob/master/jenkins/job-configs/kubernetes-jenkins-pull/bootstrap-pull.yaml)

   For build job, add an entry to [test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml](https://github.com/kubernetes/test-infra/blob/master/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml)

   For soak job, add an entry to [test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml](https://github.com/kubernetes/test-infra/blob/master/jenkins/job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml)

   For maintenance job, add an entry to [test-infra/jenkins/job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml]()

2. Add foo.sh to [test-infra/jobs](https://github.com/kubernetes/test-infra/tree/master/jobs), which defines your job behavior. You can reference it from other jobs.

3. Add your new job to [test-infra/testgrid/config/config.yaml](https://github.com/kubernetes/test-infra/blob/master/testgrid/config/config.yaml)

4. Make sure all presubmit tests pass.

