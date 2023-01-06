# Testgrid Configurations

This readme covers information specific to testgrid.k8s.io and this repository.
See Testgrid's [config.md](/testgrid/config.md) for more information Testgrid config files.

## Adding a Prow Job to Testgrid

Prow Jobs in this repository only need to be [annotated](/testgrid/config.md#prow-job-configuration);
no changes are necessary here unless you are adding a brand new dashboard.

## Adding or Changing a Configuration

Any file put in this directory or a subdirectory will be picked up by [testgrid.k8s.io](https://testgrid.k8s.io).

To add a new test, perform the following steps under any `.yaml` file in this
directory (including a new one, if desired):

1.   If writing a presubmit and not using [annotations](/testgrid/config.md#prow-job-configuration),
     append a new testgroup under `test_groups`, and specify the name and where to get the log.
1.   Append a new `dashboard_tab` under the dashboard you would like to add the testgroup to,
     or create a new `dashboard` and assign the testgroup to the dashboard.
     * The testgroup name from a dashboard tab should match the name from a testgroup
     * Note that a testgroup can be within multiple dashboards.
1.   Test your new config (`go test ./config/tests`)


NOTE: If you're adding a periodic or postsubmit and don't want to specially configure your test
group, you don't need to: [Configurator](/testgrid/cmd/configurator) implicitly assume a testgroup
exists for all periodics and postsubmits. You still need to add presubmits here, and all jobs still
need to be added to a dashboard further down.

## Testing

Run `go test //config/tests` to ensure these configurations are valid.

This finds common problems such as malformed yaml, a tab referring to a
non-existent test group, a test group never appearing on any tab, etc. It also enforces some
repository-specific conventions. More details about specific tests can be found
in that directory.
