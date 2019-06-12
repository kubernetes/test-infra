# `config-bootstrapper`

`config-bootstrapper` is used to bootstrap a configuration that would be incrementally updated by the
config-updater Prow plugin.

When a set of configurations do not exist (for example, on a clean redeployment or in a disaster
recovery situation), the config-updater plugin is not useful as it can only upload incremental
updates. This tool is meant to be used in those situations to set up the config to the correct
base state and hand off ownership to the plugin for updates.

`config-bootstrapper` uses the [`updateconfig`](https://github.com/kubernetes/test-infra/tree/master/prow/plugins/updateconfig) package to update the configmaps and uses configurations from `plugins.yaml` and `config.yaml` that are specified as command line arguments to it.
