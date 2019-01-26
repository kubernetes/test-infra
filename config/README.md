# Kubernetes Project Configs

This is a central place for Kubernetes-project specific configs for other tools in this repo.

## Directory structure:

[jobs/](./jobs) : job configs for Kubernetes Prow deployment, potentially testgrid configs as well  
[tests](./tests) : validation tests for the configs

## Adding new jobs to Prow:

1. Find or create an org/repo directory under config/jobs, eg: config/jobs/kubernetes-sigs/kustomize for jobs related to https://github.com/kubernetes-sigs/kustomize.

1. Create an OWNERS file and add appropriate approver/reviewer for your job.

1. Add a *.yaml file (the base name has to be unique), and follow https://github.com/kubernetes/test-infra/tree/master/prow#how-to-add-new-jobs for adding new prowjobs.

Also please read [`create-a-new-job`]

[`create-a-new-job`]: /README.md#create-a-new-job
