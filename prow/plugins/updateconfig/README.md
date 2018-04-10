

`updateconfig` allows prow to update configmaps when files in a repo change.

## Usage

Update your `plugins.yaml` file to something along the following lines:
```
plugins:
  my-github/repo:
  - config-updater

config_updater:
  maps:
    # Update the whatever configmap whenever thing changes
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
```
