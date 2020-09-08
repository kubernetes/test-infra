

`updateconfig` allows prow to update configmaps when files in a repo change.

`updateconfig` also supports glob match, or multi-key updates.

## Usage

Update your `plugins.yaml` file to something along the following lines:
```
plugins:
  my-github/repo:
  - config-updater

config_updater:
  maps:
    # Update the thing-config configmap whenever thing changes
    path/to/some/other/thing:
      name: thing-config
      # Using ProwJobNamespace by default.
    path/to/some/other/thing2:
      name: thing2-config
      # DEPRECATED: Please use "clusters" below
      namespace: otherNamespace
      # specify the clusters and namespaces that the configmap targets
      # which requires that the --kubeconfig arg is enabled for Hook
      # https://github.com/kubernetes/test-infra/blob/master/prow/getting_started_deploy.md#run-test-pods-in-different-clusters
      # if not set or empty, it uses the cluster where prow components are running
      # and the specified namespace(s)
      # if defined, the above namespace will be ignored
      clusters: 
        others:
        - namespace1
    # Update the config configmap whenever config.yaml changes
    config/prow/config.yaml:
      name: config
    # Update the plugin configmap whenever plugins.yaml changes
    config/prow/plugins.yaml:
      name: plugin
    # Update the `this` or/and `that` key in the `data` configmap whenever `data.yaml` or/and `other-data.yaml` changes
    some/data.yaml:
      name: data
      key: this
    some/other-data.yaml
      name: data
      key: that
    # Update the fejtaverse configmap whenever any `.yaml` file under `fejtaverse` changes
    fejtaverse/**/*.yaml
      name: fejtaverse
```
