# Autobumper

This tool automates the version upgrading of images such as the [prow.k8s.io](https://prow.k8s.io) Prow deployment.
Its workflow is:

* Given a local git repo containing the manifests of Prow component deployment,
    e.g., [/config/prow/cluster](https://github.com/kubernetes/test-infra/tree/master/config/prow/cluster) folder in this repo.
* Find out the most recent tags of given prefixes in `gcr.io` registry
    and modify the yaml files with them.
* `git-commit` the change, push it to the remote repo, and create/update a PR,
    e.g., [test-infra/pull/14249](https://github.com/kubernetes/test-infra/pull/14249), for the change.

The cluster admins can upgrade the version of images by approving the PR.

Define Prow jobs to utilize this tool:

* Periodic job for the above workflow: Periodically generate PRs for bumping the version,
    e.g., [ci-test-infra-autobump-prow](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L869).
* Postsubmit job for auto-deployment: In order to make the changes effective in Prow-cluster,
a postsubmit job, e.g., [`post-test-infra-deploy-prow`](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L89)
    for [prow.k8s.io](https://prow.k8s.io/) is defined for deploying the yaml files.

### Requirement
We need to fulfil those requirements to use this tool:

* a "committable" local repo, i.e., `git-commit` command can be executed successfully, e.g., `git-config` is set up correctly.
    This can be achieved by clone the repo by `extra_refs`, e.g.,

    ```yaml
      extra_refs:
      - org: kubernetes
        repo: test-infra
        base_ref: master
    ```

* a [GitHub token](https://help.github.com/en/articles/creating-a-personal-access-token-for-the-command-line) which has permissions
    to be used by this tool to push changes and create PRs against the remote repo.

* a yaml config file that specifies the follwing information passed in with the flag -config=FILEPATH:
  * For info about what should go in the config look at [the documentation for the Options here](https://pkg.go.dev/k8s.io/test-infra/experiment/autobumper/bumper#Options) and look at the example below.
  
e.g.,
```yaml
gitHubLogin: "k8s-ci-robot"
gitHubToken: "/etc/github-token/oauth"
gitName: "Kubernetes Prow Robot"
gitEmail: "k8s.ci.robot@gmail.com"
onCallAddress: "https://storage.googleapis.com/kubernetes-jenkins/oncall.json"
skipPullRequest: false
gitHubOrg: "kubernetes"
gitHubRepo: "test-infra"
remoteName: "test-infra"
upstreamURLBase: "https://raw.githubusercontent.com/kubernetes/test-infra/master"
includedConfigPaths:
  - "."
excludedConfigPaths:
  - "config/prow-staging"
extraFiles:
  - "config/jobs/kubernetes/kops/build-grid.py"
  - "config/jobs/kubernetes/kops/build-pipeline.py"
  - "releng/generate_tests.py"
  - "images/kubekins-e2e/Dockerfile"
targetVersion: "latest"
prefixes:
  - name: "Prow"
    prefix: "gcr.io/k8s-prow/"
    refConfigFile: "config/prow/cluster/deck_deployment.yaml"
    stagingRefConfigFile: "config/prow-staging/cluster/deck_deployment.yaml"
    repo: "https://github.com/kubernetes/test-infra"
    summarise: true
    consistentImages: true
  - name: "Boskos"
    prefix: "gcr.io/k8s-staging-boskos/"
    refConfigFile: "config/prow/cluster/boskos.yaml"
    stagingRefConfigFile: "config/prow-staging/cluster/boskos.yaml"
    repo: "https://github.com/kubernetes-sigs/boskos"
    summarise: false
    consistentImages: true
  - name: "Prow-Test-Images"
    prefix: "gcr.io/k8s-testimages/"
    repo: "https://github.com/kubernetes/test-infra"
    summarise: false
    consistentImages: false
```

