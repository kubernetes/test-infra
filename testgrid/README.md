# Testgrid

NOTE: Most TestGrid code is moving to its new home in
https://github.com/GoogleCloudPlatform/testgrid. See
[#10409](https://github.com/kubernetes/test-infra/issues/10409) for more.

The Kubernetes Testgrid instance is available at https://testgrid.k8s.io

We have a short [video] from the testgrid session at the 2018 contributor summit.

The video demos power features of testgrid, including:
* Sorting
* Filtering
* Graphing
* Grouping
* Dashboard groups
* Summaries

Please have a look!

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

## Clustered Failures

You can display identified clustered failures in your test results grid in a
dashboard tab. Select the ***Display Clustered Failures List*** toggle button to
render a list/table of identified failure clusters at the bottom of the browser.

Clusters can be grouped by:
* test status
* test status and error message

The clustered failures table shows the test status, error message (if grouped by
error message), and area of the clusters. The clusters are sorted by area in
descending order.

Selecting a row highlights the cells belonging to that cluster. Multiple row
selection (with multiple cluster highlighting) is supported. To de-select a row,
click on the selected row again.

## Configuration

If you need to add a new test that you want TestGrid to display, or otherwise change what is shown
on https://testgrid.k8s.io, see [Testgrid Configuration](config.md).

Updates to the config are automatically tested and pushed to production.

## Contributing

If you want to modify TestGrid beyond adding new tests or dashboards, see [Updating Testgrid](build_test_update.md).

[video]: https://www.youtube.com/watch?v=jm2l2SLq_yE
