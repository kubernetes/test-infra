# Config Merger

TestGrid is running a [Config Merger](https://github.com/GoogleCloudPlatform/testgrid/tree/master/cmd/config_merger)
to combine
configurations from multiple Prow instances. You can have your Prow results
appear in TestGrid via Config Merger.

1. Add Configurator Prow Job to _your_ Prow instance. You can use the
[example prowjobs](./config-merger-prowjob-example.yaml) as a template.

2. Add Configurator's cloud location to the [mergelists](/config/mergelists).
