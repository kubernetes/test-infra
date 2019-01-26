# Getting more out of Prow

If you want more functionality from your Prow instance this guide is for you. It primarily links to other resources that catalogue existing components and features.

## Use more Prow components and plugins

Prow has a number of optional cluster components and a suite of plugins for `hook` that provide all sorts of automation. Check out the [README](/prow/cmd/README.md) in the [`prow/cmd`](/prow/cmd) directory for a list of cluster components and the [README](/prow/plugins/README.md) in the [`prow/plugins`](/prow/plugins) directory for information about available plugins.

## Consume Prometheus metrics

Some Prow components expose prometheus metrics that can be used for monitoring, alerting, and pretty graphs. You can find details in the [README](/prow/metrics/README.md) in the [`prow/metrics`](/prow/metrics) directory.

## Use other tools with Prow

* If you find that your GitHub bot is running low on API tokens consider using [`ghproxy`](/ghproxy) to cache requests to GitHub and take advantage of the strange re-validation rules that allow for additional API token savings.
* [Testgrid](/testgrid) provides a highly configurable visual overview of test results and can be configured to send alerts for failing or stale results. Testgrid is in the process of being open sourced, but until it has completely made the switch OSS users will need to use the https://testgrid.k8s.io instance that is managed by the GKE-Engprod team.

## Handle scale

If your Prow instance operates on a lot of GitHub repos or runs lots of jobs you should review the ["Scaling Prow"](/prow/scaling.md) guide for tips and best practices.
