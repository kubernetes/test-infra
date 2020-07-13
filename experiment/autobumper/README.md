# Autobumper

This tool automates the version upgrading of images for the [prow.k8s.io](https://prow.k8s.io) Prow deployment.
Its workflow is:

* Given a local git repo containing the manifests of Prow component deployment,
    e.g., [/config/prow/cluster](https://github.com/kubernetes/test-infra/tree/master/config/prow/cluster) folder in this repo.
* Find out the most recent tags of "Prow images" and "k8s-testimages" in `gcr.io` registry
    and modify the yaml files with them.
* `git-commit` the change, push it to the remote repo, and create/update a PR,
    e.g., [test-infra/pull/14249](https://github.com/kubernetes/test-infra/pull/14249), for the change.

The Prow cluster admins can upgrade the version of images by approving the PR.

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

### Automated version bumping for other Prow clusters

The package `k8s.io/test-infra/experiment/autobumper/bumper` provides APIs to
bumper other Prow deployments if for some reason, such as [_update bazel config_](https://github.com/kubernetes/test-infra/blob/master/experiment/autobumper/main.go#L90)
is not needed, this tool cannot be used directly.
