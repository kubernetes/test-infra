# Developing and Contributing to Prow

## Contributing

Please consider upstreaming any changes or additions you make! Contributions in any form (issues, pull requests, even constructive comments in discussions) are more than welcome!
You can develop in-tree for more help and review, or out-of-tree if you need to for whatever reason. If you upstream a new feature or a change that impacts the default behavior of Prow, consider adding an [announcement](./ANNOUNCEMENTS.md) about it and dropping an email at the [sig-testing](https://groups.google.com/forum/#!forum/kubernetes-sig-testing) mailing list.

**New Contributors** should search for issues in kubernetes/test-infra with the `help-wanted` and/or `good first issue` labels. [(Query link)](https://github.com/kubernetes/test-infra/issues?utf8=%E2%9C%93&q=is%3Aopen+is%3Aissue+label%3A%22help+wanted%22). Before starting work please ensure that the issue is still active and then provide a short design overview of your planned solution.
Also reach out on the Kubernetes slack in the `sig-testing` channel.

## Prow Integration Points

There are a number of ways that you can write code for Prow or integrate existing code with Prow.

#### Plugins

[Prow plugins](/prow/plugins) are sub-components of the [`hook`](/prow/cmd/hook) binary that register event handlers for various types of GitHub events.
Plugin event handlers are provided a [`PluginClient`](https://godoc.org/k8s.io/test-infra/prow/plugins#PluginClient) that provides access to a suite of clients and agents for configuration, ProwJobs, GitHub, git, OWNERS file, Slack, and more.

##### How to add new plugins

Add a new package under `plugins` with a method satisfying one of the handler
types in `plugins`. In that package's `init` function, call
`plugins.Register*Handler(name, handler)`. Then, in `hook/plugins.go`, add an
empty import so that your plugin is included. If you forget this step then a
unit test will fail when you try to add it to `plugins.yaml`. Don't add a brand
new plugin to the main `kubernetes/kubernetes` repo right away, start with
somewhere smaller and make sure it is well-behaved.

The [`lgtm` plugin](/prow/plugins/lgtm) is a good place to start if you're looking for an example
plugin to mimic.

##### External plugins

For even more flexibility, *anything* that receives GitHub webhooks can be configured to be forwarded webhooks as an [external plugin](/prow/plugins/README.md#external-plugins). This allows in-cluster or out of cluster plugins and forwarding to other bots or infrastructure.

#### Cluster Deployments

Additional cluster components can use the informer framework for ProwJobs in order to react to job creation, update, and deletion.
This can be used to implement additional job execution controllers for executing job with different agents. For example, `jenkins-operator` executes jobs on jenkins, `plank` uses kubernetes pods, and `build` uses the build CRD.
The informer framework can also be used to react to job completion or update in order to create an alternative job reporting mechanism.

#### Artifact Viewers

[Spyglass](/prow/spyglass) artifact viewers allow for custom display of ProwJob artifacts that match a certain file regexp. Existing viewers display logs, metadata, and structured junit results.

#### ProwJobs

[ProwJobs](/prow/jobs.md) themselves are often a sufficient integration point if you just need to execute a task on a schedule or in reaction to code changes.

#### Exposed Data

If you just need some data from Prow you may be able to get it from the JSON exposed by Prow's front end `deck`, or from Prometheus metrics.


## Building, Testing, and Deploying:

You can build, test, and deploy Prow’s binaries, container images, and cluster resources using [`bazel`](https://bazel.build). See [`getting_started_deploy.md`](/prow/getting_started_deploy.md) for initially deploying Prow and [`build_test_update.md`](/prow/build_test_update.md) for iterating on an existing deployment.

