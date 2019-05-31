# Plugins

Plugins are sub-components of [`hook`](/prow/cmd/hook) that consume [GitHub webhooks](https://developer.github.com/webhooks/) related to their function and can be individually enabled per repo or org.

All plugin specific configuration is stored in [`plugins.yaml`](/prow/plugins.yaml).
The `Configuration` golang struct holds all the config fields organized into substructures by plugin. See it's [GoDoc](https://godoc.org/k8s.io/test-infra/prow/plugins#Configuration) for up-to-date descriptions of every config option.

## Help Information

Most plugins lack README's but instead generate `PluginHelp` structs on demand that include general explanations and help information in addition to details about the current configuration.

Please see https://prow.k8s.io/plugins for a list of all plugins deployed on the Kubernetes Prow instance, what they do, and what commands they offer.
For an alternate view, please see https://prow.k8s.io/command-help to see all of the commands offered by the deployed plugins.

## How to enable a plugin on a repo

Add an entry to [plugins.yaml](/prow/plugins.yaml). If you misspell the name then a
unit test will fail. If you have [update-config](/prow/plugins/updateconfig) plugin
deployed then the config will be automatically updated once the PR is merged,
else you will need to run `make update-plugins`. This does not require
redeploying the binaries, and will take effect within a minute.

## External Plugins

External plugins offer an alternative to compiling a plugin into the `hook` binary. Any web endpoint that can properly handle GitHub webhooks can be configured as an external plugin that `hook` will forward webhooks to. External plugin endpoints are specified per org or org/repo in [`plugins.yaml`](/prow/plugins.yaml) under the `external_plugins` field. Specific event types may be optionally specified to filter which events are forwarded to the endpoint.
External plugins are well suited for:
- Slow operations that would impact the performance of other plugins if run as part of `hook`.
- Components that need to be triggered or notified of events beside GitHub webhooks.
- Isolating a more or less privileged plugin or a plugin that executes PR code.
- Integrating existing GitHub services with Prow.

Examples of external plugins can be found in the [`prow/external-plugins`](/prow/external-plugins) directory. The following is an example external plugin configuration that would live in [`plugins.yaml`](/prow/plugins.yaml).
```yaml
external_plugins:
  org-foo/repo-bar:
  - name: refresh-remote
    endpoint: https://my-refresh-plugin.com
    events:
    - issue_comment
  - name: needs-rebase
    # No endpoint specified implies "http://{{name}}".
    events:
    - pull_request
  - name: cherrypick
    # No events specified implies all event types.
```

## How to test a plugin

See [`build_test_update.md`](/prow/build_test_update.md#How-to-test-a-plugin).
