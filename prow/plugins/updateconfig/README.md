

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
      namespace: otherNamespace
    # Update the config configmap whenever config.yaml changes
    prow/config.yaml:
      name: config
    # Update the plugin configmap whenever plugins.yaml changes
    prow/plugins.yaml:
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
