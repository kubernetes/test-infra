# Getting more out of Prow

If you want more functionality from your Prow instance this guide is for you. It primarily links to other resources that catalogue existing components and features.

## Use more Prow components and plugins

Prow has a number of optional cluster components and a suite of plugins for `hook` that provide all sorts of automation. Check out the [README](/prow/cmd/README.md) in the [`prow/cmd`](/prow/cmd) directory for a list of cluster components and the [README](/prow/plugins/README.md) in the [`prow/plugins`](/prow/plugins) directory for information about available plugins.

## Consume Prometheus metrics

Some Prow components expose prometheus metrics that can be used for monitoring, alerting, and pretty graphs. You can find details in the [README](/prow/metrics/README.md) in the [`prow/metrics`](/prow/metrics) directory.

## Make Prow update and deploy itself!

You can easily make your Prow instance automatically update itself when changes
are made to its component's kubernetes resource files. This is achieved with a
postsubmit job that `kubectl apply`s the resource files whenever they are
changed (based on a `run_if_changed` regexp). In order to `kubectl apply` to the
cluster, the job will need to supply credentials (e.g. a kubeconfig file or
[GCP service account key-file](/prow/gcloud-deployer-service-account.sh)). Since
this job requires priviledged credentials to deploy to the cluster, it is
important that it is run in a separate build cluster that is isolated from all
presubmit jobs. See the
[documentation about separate build clusters](/prow/scaling.md#separate-build-clusters)
for details. An example of such a job can be found
[here](https://github.com/istio/test-infra/blob/45526926b4f1cd09147d54d23abc4a4258e62860/prow/cluster/jobs/istio/test-infra/istio.test-infra.trusted.master.yaml#L2-L28).
Once you have a postsubmit deploy job, any changes to Prow component files are
automatically applied to the cluster when the changes merge. In order to ensure
that all changes to production are properly approved, you can use OWNERS files
with the [`approve` plugin](/prow/plugins/approve) and [`Tide`](/prow/cmd/tide).

With the help of the [Prow Autobump utility](/prow/cmd/autobump#prow-autobump) you can easily create commits that update all references to Prow images to the latest image version that has been vetted by the https://prow.k8s.io instance. If your Prow component resource files live in GitHub, this utility can even automatically create/update a Pull Request that includes these changes. This works great when run as a periodic job since it will maintain a single open PR that is periodically updated to reference the most recent upstream version. See the [README](/prow/cmd/autobump#prow-autobump) for details and an example.

Combining a postsubmit deploy job with a periodic job that runs the Prow Autobump utility allows Prow to be updated to the latest version by simply merging the automatically created Pull Request (or letting Tide merge it after it has been approved).

### Deploy config changes automatically

Prow can also automatically upload changes to files that correspond to Kubernetes ConfigMaps. This includes its own `config`, `plugins` and `job-config` config maps. Take a look at the [`updateconfig` plugin](/prow/plugins/updateconfig) and [`config-bootstrapper`](/prow/cmd/config-bootstrapper) for more details. Both of these tools share the [`updateconfig` plugin's plugin configuration](https://github.com/kubernetes/test-infra/blob/531f2a5e6b6fb60e3262340a86992029aa59808f/prow/plugins/config.go#L69). The plugin provides slightly better GitHub integration and is simpler to enable, but the config-bootstrapper is more flexible. It can be run in a postsubmit job to provide config upload on non-GitHub Prow instances or run after custom config generation is executed.

## Use other tools with Prow

* If you find that your GitHub bot is running low on API tokens consider using [`ghproxy`](/ghproxy) to cache requests to GitHub and take advantage of the strange re-validation rules that allow for additional API token savings.
* [Testgrid](/testgrid) provides a highly configurable visual overview of test results and can be configured to send alerts for failing or stale results. Testgrid is in the process of being open sourced, but until it has completely made the switch OSS users will need to use the https://testgrid.k8s.io instance that is managed by the GKE-Engprod team.
* [Kind](https://github.com/kubernetes-sigs/kind) lets you run an entire Kubernetes cluster in a container. This makes it fast and easy for ProwJobs to test anything that runs on Kubernetes (or Kubernetes itself).
* [label_sync](/label_sync) maintains GitHub labels across orgs and repos based on yaml configuration.

## Handle scale

If your Prow instance operates on a lot of GitHub repos or runs lots of jobs you should review the ["Scaling Prow"](/prow/scaling.md) guide for tips and best practices.
