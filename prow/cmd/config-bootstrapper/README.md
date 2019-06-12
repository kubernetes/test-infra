# `config-bootstrapper`

`config-bootstrapper` is used to bootstrap a configuration that would be incrementally updated by the
config-updater Prow plugin.

When a set of configurations do not exist (for example, on a clean redeployment or in a disaster
recovery situation), the config-updater plugin is not useful as it can only upload incremental
updates. This tool is meant to be used in those situations to set up the config to the correct
base state and hand off ownership to the plugin for updates.

Provide the config-bootstrapper with the latest state of the Prow configuration (plugins.yaml, config.yaml, any job configuration files) to boot-strap with the latest configuration.

Sample usage:
```
./config-bootstrapper \
    --dry-run=false \
    --source-path=.  \
    --config-path=prowconfig/config.yaml \
    --plugin-config=prowconfig/plugins.yaml \
    --job-config-path=prowconfig/jobs
```
