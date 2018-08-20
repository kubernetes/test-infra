# Prow Images

This directory includes a sub directory for every Prow component and is where all binary and container images are built. You can find the `main` packages here in addition to the `BUILD.bazel` files that contain [Bazel](https://bazel.build) rules for building binary and container images. For details about building the binaries and images see [`build_test_update.md`](/prow/build_test_update.md).

## Cluster Components

Prow has a microservice architecture implemented as a collection of container images that run as Kubernetes deployments. A brief description of each service component is provided here.

#### Core Components

* [`hook`](/prow/cmd/hook) is the most important piece. It is a stateless server that listens for GitHub webhooks and dispatches them to the appropriate plugins. Hook's plugins are used to trigger jobs, implement 'slash' commands, post to Slack, and more. See the [`prow/plugins`](/prow/plugins/) directory for more information on plugins.
* [`plank`](/prow/cmd/plank) is the controller that manages the job execution and lifecycle for jobs that run in k8s pods.
* [`deck`](/prow/cmd/deck) presents a nice view of [recent jobs]](https://prow.k8s.io/), [command](https://prow.k8s.io/command-help) and [plugin](https://prow.k8s.io/plugins) help information, the [status of merge automation](https://prow.k8s.io/tide), and a [dashboard for PR authors](https://prow.k8s.io/pr).
* [`horologium`](/prow/cmd/horologium) triggers periodic jobs when necessary.
* [`sinker`](/prow/cmd/sinker) cleans up old jobs and pods.

#### Merge Automation

* [`tide`](/prow/cmd/tide) manages retesting and merging PRs once they meet the configured merge criteria. See [its README](./cmd/tide/README.md) for more information.

#### Auxiliary Components

Hopefully you won't need any of these components...

* [`jenkins-operator`](/prow/cmd/jenkins-operator) is the controller that manages jobs that run on Jenkins. We moved away from using this component in favor of running all jobs on Kubernetes.
* [`tot`](/prow/cmd/tot) vends sequential build numbers. Tot is only necessary for integration with automation that expects sequential build numbers. If Tot is not used, Prow automatically generates build numbers that are monotonically increasing, but not sequential.

## Dev Tools
* [`checkconfig`](/prow/cmd/checkconfig) loads and verifies the configuration, useful as a pre-submit.
* [`mkpj`](/prow/cmd/mkpj) creates `ProwJobs` using Prow configuration.
* [`mkpod`](/prow/cmd/mkpod) creates `Pods` from `ProwJobs`.
* [`phony`](/prow/cmd/phony) sends fake webhooks for testing hook and plugins.

## Pod Utilities

These are small tools that are automatically added to ProwJob pods for jobs that request pod decoration. They are used to transparently provide source code cloning and upload of metadata, logs, and job artifacts to persistent storage. See [their README](/prow/pod-utilities.md) for more information.

* [`clonerefs`](/prow/cmd/clonerefs)
* [`initupload`](/prow/cmd/initupload)
* [`entrypoint`](/prow/cmd/entrypoint)
* [`sidecar`](/prow/cmd/sidecar)

## Base Images

The container images in [`images`](/prow/cmd/images) are used as base images for Prow components.
