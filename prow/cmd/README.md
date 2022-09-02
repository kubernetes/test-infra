# Prow Images

This directory includes a sub directory for every Prow component and is where all binary and container images are built. You can find the `main` packages here. For details about building the binaries and images see [`build_test_update.md`](/prow/build_test_update.md).

## Cluster Components

Prow has a microservice architecture implemented as a collection of container images that run as Kubernetes deployments. A brief description of each service component is provided here.

#### Core Components

* [`crier`](/prow/cmd/crier) reports on ProwJob status changes. Can be configured to report to gerrit, github, pubsub, slack, etc.
* [`deck`](/prow/cmd/deck) presents a nice view of [recent jobs](https://prow.k8s.io/), [command](https://prow.k8s.io/command-help) and [plugin](https://prow.k8s.io/plugins) help information, the [current status](https://prow.k8s.io/tide) and [history](https://prow.k8s.io/tide-history) of merge automation, and a [dashboard for PR authors](https://prow.k8s.io/pr).
* [`hook`](/prow/cmd/hook) is the most important piece. It is a stateless server that listens for GitHub webhooks and dispatches them to the appropriate plugins. Hook's plugins are used to trigger jobs, implement 'slash' commands, post to Slack, and more. See the [`prow/plugins`](/prow/plugins/) directory for more information on plugins.
* [`horologium`](/prow/cmd/horologium) triggers periodic jobs when necessary.
* [`prow-controller-manager`](/prow/cmd/prow-controller-manager) manages the job execution and lifecycle for jobs that run in k8s pods. It currently acts as a replacement for [`plank`](/prow/plank)
* [`sinker`](/prow/cmd/sinker) cleans up old jobs and pods.

#### Merge Automation

* [`tide`](/prow/cmd/tide) manages retesting and merging PRs once they meet the configured merge criteria. See [its README](./tide/README.md) for more information.

#### Optional Components

* [`branchprotector`](/prow/cmd/branchprotector) configures [github branch protection] according to a specified policy
* [`exporter`](/prow/cmd/exporter) exposes metrics about ProwJobs not directly related to a specific Prow component
* [`gerrit`](/prow/cmd/gerrit) is a Prow-gerrit adapter for handling CI on [gerrit] workflows
* [`hmac`](/prow/cmd/hmac) updates HMAC tokens, GitHub webhooks and HMAC secrets for the orgs/repos specified in the Prow config file
* [`jenkins-operator`](/prow/cmd/jenkins-operator) is the controller that manages jobs that run on Jenkins. We moved away from using this component in favor of running all jobs on Kubernetes.
* [`tot`](/prow/cmd/tot) vends sequential build numbers. Tot is only necessary for integration with automation that expects sequential build numbers. If Tot is not used, Prow automatically generates build numbers that are monotonically increasing, but not sequential.
* [`status-reconciler`](/prow/cmd/status-reconciler) ensures changes to blocking presubmits in Prow configuration does not cause in-flight GitHub PRs to get stuck
* [`sub`](/prow/cmd/sub) listen to Cloud Pub/Sub notification to trigger Prow Jobs.

## CLI Tools

* [`checkconfig`](/prow/cmd/checkconfig) loads and verifies the configuration, useful as a pre-submit.
* [`config-bootstrapper`](/prow/cmd/config-bootstrapper) bootstraps a configuration that would be incrementally updated by the [`updateconfig` Prow plugin]
* [`generic-autobumper`](/prow/cmd/generic-autobumper) automates image version upgrades (e.g. for a Prow deployment) by opening a PR with images changed to their latest version according to a config file.
* [`invitations-accepter`](/prow/cmd/invitations-accepter) approves all pending GitHub repository invitations
* [`mkpj`](/prow/cmd/mkpj) creates `ProwJobs` using Prow configuration.
* [`mkpod`](/prow/cmd/mkpod) creates `Pods` from `ProwJobs`.
* [`peribolos`](/prow/cmd/peribolos) manages GitHub org, team and membership settings according to a config file. Used by [kubernetes/org]
* [`phaino`](/prow/cmd/phaino) runs an approximation of a ProwJob on your local workstation
* [`phony`](/prow/cmd/phony) sends fake webhooks for testing hook and plugins.

## Pod Utilities

These are small tools that are automatically added to ProwJob pods for jobs that request pod decoration. They are used to transparently provide source code cloning and upload of metadata, logs, and job artifacts to persistent storage. See [their README](/prow/pod-utilities.md) for more information.

* [`clonerefs`](/prow/cmd/clonerefs)
* [`initupload`](/prow/cmd/initupload)
* [`entrypoint`](/prow/cmd/entrypoint)
* [`sidecar`](/prow/cmd/sidecar)

## Base Images

The container images in [`images`](/images) are used as base images for Prow components.

## TODO: undocumented

* [`admission`](/prow/cmd/admission)
* [`gcsupload`](/prow/cmd/gcsupload)
* [`grandmatriarch`](/prow/cmd/grandmatriarch)
* [`pipeline`](/prow/cmd/pipeline)
* [`tackle`](/prow/cmd/tackle)

## Deprecated

* [`cm2kc`](/prow/cmd/cm2kc) is a CLI tool used to convert a [clustermap file][clustermap docs] to a [kubeconfig file][kubeconfig docs]. Deprecated because we have moved away from clustermaps; you should use [`gencred`] to generate a [kubeconfig file] directly.

<!-- links -->

[github branch protection]: https://help.github.com/articles/about-protected-branches/
[clustermap docs]: https://github.com/kubernetes/test-infra/blob/1c7d9a4ae0f2ae1e0c11d8357f47163d18521b84/prow/getting_started_deploy.md#run-test-pods-in-different-clusters
[kubeconfig docs]: https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/
[`gencred`]: /gencred
[gerrit]: https://www.gerritcodereview.com/
[`updateconfig` Prow plugin]: /prow/plugins/updateconfig
[kubernetes/org]: https://github.com/kubernetes/org
