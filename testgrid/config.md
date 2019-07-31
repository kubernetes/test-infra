# TestGrid Configuration

## Table of Contents

* [Prow Job Configuration](#prow-job-configuration)
* [Configuration](#configuration)
* [Testing & Verification](#testing-your-configuration)
* [Advanced Configuration](#advanced-configuration)


Testgrid is composed of:
* A list of **test groups** that contain results for a job over time.
* A list of **dashboard tabs** that display a test group
* A list of **dashboards**, or collections of dashboard tabs
* A list of **dashboard groups** of related dashboards.

Most of these objects are simply listed in a [YAML config file][configuration] for Testgrid to consume.

## Prow Job Configuration

If you just have a [Prow job](/prow/jobs.md) configuration you want to appear in an existing
dashboard, add annotations to that Prow job.

If it's a Prow job in [the k8s.io instance](/config/jobs), you don't need to do anything else.


Add this to your Prow job:

```yaml
annotations:
  testgrid-dashboards: dashboard-name      # a dashboard already defined in a config.yaml.
  testgrid-tab-name: some-short-name       # optionally, a shorter name for the tab. If omitted, just uses the job name.
  testgrid-alert-email: me@me.com          # optionally, an alert email that will be applied to the tab created in the
                                           # first dashboard specified in testgrid-dashboards.
  description: Words about your job.       # optionally, a description of your job. If omitted, just uses the job name.

  testgrid-num-columns-recent: "10"        # optionally, the number of runs a row can be omitted from before it is
                                           # considered stale. Currently defaults to 10.
  testgrid-num-failures-to-alert: "3"      # optionally, the number of continuous failures before sending an email.
                                           # Currently defaults to 3.
  testgrid-alert-stale-results-hours: "12" # optionally, send an email if this many hours pass with no results at all.

```

This functionality is provided by [Configurator](cmd/configurator). If you have Prow jobs in a _different_
instance of Prow, you may want to invoke Configurator [differently](cmd/configurator#deserialization-options).

If you need to create a new dashboard, or do anything more advanced, read on.

## Configuration
Open or create a Testgrid config file [(example)][configuration] in your favorite editor and:
1. Configure the test groups
2. Add those testgroups to one or more tabs in one or more dashboards
3. Consider using dashboard groups if multiple dashboards are needed.

### Test groups
Test groups contain a set of test results across time for the same job.
Each group backs one or more dashboard tabs.

Add a new test group under `test_groups:`, specifying the group's name,
and where the logs are located.

Ex:

```
test_groups:
- name: {test_group_name}
  gcs_prefix: kubernetes-jenkins/logs/{test_group_name}
```

See the `TestGroup` message in [`config.proto`] for additional fields to
configure like `days_of_results`, `tests_name_policy`, `notifications`, etc.

### Dashboards
#### Tabs
A dashboard tab is a particular view of a test group. Multiple dashboard tabs can view the same
test group in different ways, via different configuration options. All dashboard tabs belong under
a dashboard (see below).

#### Dashboards

A dashboard is a set of related dashboard tabs.  The dashboard name shows up as the top-level link
when viewing TestGrid.

Add a new dashboard under `dashboards` and a new dashboard tab under that.

Ex:

```
dashboards:
- name: {dashboard-name}
  dashboard_tab:
  - name: {dashboard-tab-name}
    test_group_name: {test-group-name}
```

See the `Dashboard` and `DashboardTab` messages in [`config.proto`] for
additional configuration options, such as `notifications`, `file_bug_template`,
`description`, `code_search_url_template`, etc.

#### Dashboard groups
A dashboard group is a set of related dashboards. When viewing a dashboard's tabs, you'll see the
other dashboards in the Dashboard Group at the top of the client.

Add a new dashboard group, specifying names for all the dashboards that fall under this group.

Ex:

```
dashboard_groups:
- name: {dashboard-group-name}
  dashboard_names:
  - {dashboard-1}
  - {dashboard-2}
  - {dashboard-3}
```

## Testing your configuration

Run `bazel test //testgrid/...` to ensure the configuration is valid.

This finds common problems such as malformed yaml, a tab referring to a
non-existent test group, a test group never appearing on any tab, etc.

## Advanced configuration
See [`config.proto`] for an extensive list of configuration options. Here are some commonly-used ones.

### More/Fewer Results
Specify `days_of_results` in a test group to increase or decrease the number of days of results shown.

```
test_groups:
- name: kubernetes-build
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-build
  days_of_results: 7
```

### Tab descriptions
Add a short description to a dashboard tab describing its purpose.

```
  dashboard_tab:
  - name: gce
    test_group_name: ci-kubernetes-e2e-gce
    base_options: 'include-filter-by-regex=Kubectl%7Ckubectl'
    description: 'kubectl gce e2e tests for master branch'
```

### Column headers
TestGrid shows date, build number, and k8s and test-infra commit shas above
each run's results by default. To add your own custom column headers, add a
key-value pair in your tests' metadata (see [metadata for
finished.json](https://github.com/kubernetes/test-infra/tree/master/gubernator#job-artifact-gcs-layout)),
and add the key for that pair as a `configuration_value` under `column_header`
for your test group. Example:

```
test_groups:
- name: ci-kubernetes-e2e-gce-ubuntudev-k8sdev-default
  gcs_prefix:
  kubernetes-jenkins/logs/ci-kubernetes-e2e-gce-ubuntudev-k8sdev-default
  column_header:
  - configuration_value: node_os_image
  - configuration_value: master_os_image
  - configuration_value: Commit
  - configuration_value: infra-commit
```

### Email alerts
In TestGroup, set `num_failures_to_alert` (alerts for consistent failures)
and/or `alert_stale_results_hours` (alerts when tests haven't run recently).
You can also set `num_passes_to_disable_alert`.

In DashboardTab, set `alert_mail_to_addresses` (comma-separated list of email
addresses to send mail to).

Additional options for DashboardTab alerts:
- `num_passes_to_disable_alert`: the number of consecutive test passes to close the alert
- `subject`: custom subject for alert mails
- `debug_url`: custom link for further context/instructions on debugging this alert
- `debug_message`: custom text to show for the debug link; `debug_url` is required for `debug_message` to appear

These alerts will send whenever new failures are detected (or whenever the
dashboard tab goes stale), and will stop when `num_passes_to_disable_alert`
consecutive passes are found (or no failure is found in `num_columns_recent`
runs).

```
# Send alerts to foo@bar.com whenever a test fails 3 times in a row, or tests
# haven't run in the last day.
test_groups:
- name: ci-kubernetes-e2e-gce
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-e2e-gce
  alert_stale_results_hours: 24
  num_failures_to_alert: 3

dashboards:
- name: google-gce
  dashboard_tab:
  - name: gce
    test_group_name: ci-kubernetes-e2e-gce
    alert_options:
      alert_mail_to_addresses: 'foo@bar.com'
```


### Base options
Default to a set of client modifiers when viewing this dashboard tab.

```
# Show test cases from ci-kubernetes-e2e-gce, but only if the test has 'Kubectl' or 'kubectl' in the name.
  dashboard_tab:
  - name: gce
    test_group_name: ci-kubernetes-e2e-gce
    base_options: 'include-filter-by-regex=Kubectl%7Ckubectl'
    description: 'kubectl gce e2e tests for master branch'
```

### More informative test names
If you run multiple versions of a test against different parameters, show which parameters they with after the test name.

```
# Show a test case as "{test_case_name} [{Context}]"
- name: ci-kubernetes-node-kubelet-benchmark
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-node-kubelet-benchmark
  test_name_config:
    name_elements:
    - target_config: Tests name
    - target_config: Context
    name_format: '%s [%s]'
```

### Customize regression search
Narrow down where to search when searching for a regression between two builds/commits.

```
  dashboard_tab:
  - name: bazel
    description: Runs bazel test //... on the test-infra repo.
    test_group_name: ci-test-infra-bazel
    code_search_url_template:
      url: https://github.com/kubernetes/test-infra/compare/<start-custom-0>...<end-custom-0>
```

### Notifications
Testgrid supports the ability to add notifications, which appears as a yellow
butter bar / toast message at the top of the screen.

This is an effective way to broadcast system wide information (all
FOO suites are failing due to blah, upgrade frobber to vX before the
weekend, etc.)

Configure the list of `notifications:` under dashboard or testgroup:
Each notification includes a `summary:` that defines the text displayed.
Notifications benefit from including a `context_link:` url that can be clicked
to provide more information.

Ex:

```
dashboards:
- name: k8s
  dashboard_tab:
  - name: build
    test_group_name: kubernetes-build
  notifications:  # Attach to a specific dashboard
  - summary: Hello world (first notification).
  - summary: Tests are failing to start (second notification).
    context_link: https://github.com/kubernetes/kubernetes/issues/123
```

or

```
test_groups:  # Attach to a specific test_group
- name: kubernetes-build
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-build
  notifications:
  - summary: Hello world (first notification)
  - summary: Tests are failing to start (second notification).
    context_link: https://github.com/kubernetes/kubernetes/issues/123
```

### What Counts as 'Recent'
Configure `num_columns_recent` to change how many columns TestGrid should consider 'recent' for results.
TestGrid uses this to calculate things like 'is this test stale?' (and hides the test).

```
test_groups:
- name: kubernetes-build
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-build
  num_columns_recent: 3
```

### Long-Running Tests
If your tests run for a very long time (more than 24 hours), set
`max_test_runtime_hours`.

```
# This test group has tests that run for 48 hours; set a high max runtime.
test_groups:
- name: some-tests
  gcs_prefix: path/to/test/logs/some-tests
  max_test_runtime_hours: 50  # Leave a small buffer just in case.
```

### Ignore Pending Results
`ignore_pending` is false by default, which means that in-progress results will
be shown if we have data for them. If you want to have these not show up, add:

```
test_groups:
- name: kubernetes-build
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-build
  ignore_pending: true
```

### Showing a metric in the cells
Specify `short_text_metric` to display a custom numeric metric in the TestGrid cells. Example:

```
test_groups:
- name: ci-kubernetes-coverage-conformance
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-coverage-conformance
  short_text_metric: coverage
```

[`config.proto`]: ./config/config.proto
[configuration]: /config/testgrids