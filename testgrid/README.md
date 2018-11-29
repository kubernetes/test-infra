# Testgrid

### Table of Contents
* [Configuration](#configuration)
* [Advanced Configuration](#advanced-configuration)
* [Using the Client](#using-the-client)
* [Unit Testing](#unit-testing)
* [Merging Changes](#merging-changes)


The testgrid site is accessible at https://testgrid.k8s.io. The site is
configured by [`config.yaml`].
Updates to the config are automatically tested and pushed to production.

Testgrid is composed of:
* A list of test groups that contain results for a job over time.
* A list of dashboards that are composed of tabs that display a test group
* A list of dashboard groups of related dashboards.

## Configuration
Open [`config.yaml`] in your favorite editor and:
1. Configure the test groups
2. Add those testgroups to one or more tabs in one or more dashboards
3. Consider using dashboard groups if multiple dashboards are needed.

### Test groups
Test groups contain a set of test results across time for the same job. Each group backs one or more dashboard tabs.

Add a new test group under `test_groups:`, specifying the group's name, and where the logs are located.

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
A dashboard tab is a particular view of a test group. Multiple dashboard tabs can view the same test group in different ways, via different configuration options. All dashboard tabs belong under a dashboard (see below).

#### Dashboards

A dashboard is a set of related dashboard tabs.  The dashboard name shows up as the top-level link when viewing TestGrid.

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
A dashboard group is a set of related dashboards. When viewing a dashboard's tabs, you'll see the other dashboards in the Dashboard Group at the top of the client.

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

## Using the client

Here are some quick tips and clarifications for using the TestGrid site!

### Tab Statuses

TestGrid assigns dashboard tabs a status based on recent test runs.

 *  **PASSING**: No failures found in recent (`num_columns_recent`) test runs.
 *  **FAILING**: One or more consistent failures in recent test runs.
 *  **FLAKY**: The tab is neither PASSING nor FAILING. There is at least one
    recent failed result that is not a consistent failure.

### Summary Widget

You can get a small widget showing the status of your dashboard tab, based on
the tab statuses above! For example:

`sig-testing-misc#bazel`: [![sig-testing-misc/bazel](https://testgrid.k8s.io/q/summary/sig-testing-misc/bazel/tests_status?style=svg)](https://testgrid.k8s.io/sig-testing-misc#bazel)

Inline it with:

```
<!-- Inline with a link to your tab -->
[![<dashboard_name>/<tab_name>](https://testgrid.k8s.io/q/summary/<dashboard_name>/<tab_name>/tests_status?style=svg)](https://testgrid.k8s.io/<dashboard_name>#<tab_name>)
```

### Customizing Test Result Sizes

Change the size of the test result rectangles.

The three sizes are Standard, Compact, and Super Compact. You can also specify
`width=X` in the URL (X > 3) to customize the width. For small widths, this may
mean the date and/or changelist, or other custom headers, are no longer
visible.

### Filtering Tests

You can repeatedly add filters to include/exclude test rows. Under **Options**:

*   **Include/Exclude Filter by RegEx**: Specify a regular expression that
    matches test names for rows you'd like to include/exclude.
*   **Exclude non-failed Tests**: Omit rows with no failing results.

### Grouping Tests

Grouped tests are summarized in a single row that is collapsible/expandable by
clicking on the test name (shown as a triangle on the left). Under **Options**:

*   **Group by RegEx Mask**: Specify a regular expression to mask a portion of
    the test name. Any test names that match after applying this mask will be
    grouped together.
*   **Group by Target**: Any tests that contain the same target will be
    grouped together.
*   **Group by Hierarchy Pattern**: Specify a regular expression that matches
    one or more parts of the tests' names and the tests will be grouped
    hierarchically. For example, if you have these tests in your dashboard:

    ```text
    /test/dir1/target1
    /test/dir1/target2
    /test/dir2/target3
    ```

    By specifying regular expression "\w+", the tests will be organized into:

    ```text
    ▼test
      ▼dir1
        target1
      ▼dir2
        target2
        target3
    ```

### Sorting Tests

Under **Options**

*   **Sort by Failures**: Tests with more recent failures will appear before
    other tests.
*   **Sort by Flakiness**: Tests with a higher flakiness score will appear
    before tests with a lower flakiness score. The flakiness score, which is not
    reported, is based on the number of transitions from passing to failing (and
    vice versa) with more weight given to more recent transitions.
*   **Sort by Name**: Sort alphabetically.

## Unit testing

Run `bazel test //testgrid/...` to ensure the config is valid.

This finds common problems such as malformed yaml, a tab referring to a
non-existent test group, a test group never appearing on any tab, etc.

Run `bazel test //...` for slightly more advanced testing, such as ensuring that
every job in our CI system appears somewhere in testgrid, etc.

All PRs updating the configuration must pass prior to merging


## Merging changes

Updates to the testgrid configuration are automatically pushed immediately when
merging a change.

Manually convert the yaml file to the config proto with:
```
bazel run //testgrid/cmd/config -- \
  --yaml=testgrid/config.yaml \
  --print-text \
  --oneshot \
  --output=/tmp/config.pb \
  # Or push to gcs
  # --output=gs://my-bucket/config
  # --gcp-service-account=/path/to/foo.json
```

[`config.proto`]: ./config.proto
[`config.yaml`]: ./config.yaml
