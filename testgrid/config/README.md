# Opensource config for testgrid.k8s.io

build :
  go build ./yaml2proto
  go install

usage:
config \<input/path/to/yaml\> \<output/path/to/proto\>

# Add a new test

----------------------------------------------------------------------------------------

## 1. Update [config.yaml](https://github.com/kubernetes/test-infra/blob/master/testgrid/config/config.yaml).

### Add a new Test Group
Test Group: Test case results over time for a job. A test group backs one or more dashboard tabs.

Add a new test group under `test_groups`, specifying the test group's name, and where the logs are located.

Ex:

```
test_groups:
- name: {test_group_name}
  gcs_prefix: kubernetes-jenkins/logs/{test_group_name}
```

### Add a new Dashboard Tab
Dashboard Tab: A particular view of a test group. Multiple dashboard tabs can view the same test group in different ways, via different configuration options. All dashboard tabs belong under a dashboard (see below).

### Add a new Dashboard
Dashboard: A set of related dashboard tabs. The dashboard name shows up as the top-level link when viewing TestGrid.

Add a new dashboard under `dashboards` and a new dashboard tab under that.

Ex:

```
dashboards:
- name: {dashboard-name}
  dashboard_tab:
  - name: {dashboard-tab-name}
    test_group_name: {test-group-name}
```

### Add a new Dashboard Group
Dashboard Group: A group of related dashboards. When viewing a dashboard's tabs, you'll see the other dashboards in the Dashboard Group at the top of the client.

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

## 2. [Optionally] Add additional configuration
See [config.proto](https://github.com/kubernetes/test-infra/blob/master/testgrid/config/config.proto) for an extensive list of configuration options. Here are some commonly-used ones.

### Tab descriptions
Add a short description to a dashboard tab describing its purpose.

```
  dashboard_tab:
  - name: gce
    test_group_name: ci-kubernetes-e2e-gce
    base_options: 'include-filter-by-regex=Kubectl%7Ckubectl'
    description: 'kubectl gce e2e tests for master branch'
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

### Add a notification
Append 'notifications' under the testgroup or dashboard, specifying 'summary' (short text description of the notice or issue), and optionally 'context_link' (fully-qualified URL with more information).

Ex:

```
test_groups:
- name: kubernetes-build
  gcs_prefix: kubernetes-jenkins/logs/ci-kubernetes-build
  notifications:
  - summary: I am a test group notification.
  - summary: Tests are failing to start.
    context_link: https://github.com/kubernetes/kubernetes/issues/123
```

or 

```
dashboards:
- name: k8s
  dashboard_tab:
  - name: build
    test_group_name: kubernetes-build
  notifications:
  - summary: I am a dashboard notification.
  - summary: Tests are failing to start.
    context_link: https://github.com/kubernetes/kubernetes/issues/123
```


## 3. Run `config_test.go` to make sure the config is valid.


# Changing `config.proto`
Contact us before you make changes to config.proto

Devs - If you changed config.proto:

1. Make sure [protoc](https://github.com/golang/protobuf) is installed and

2. `protoc --go_out=pb config.proto`

3. Search-replace all `json:"foo,omitempty"` with `yaml:"foo,omitempty"`.

   ```
   # Be sure to add back the header
   sed -i -e 's/json:/yaml:/g' pb/config.pb.go
   ```

4. Commit both `config.proto` and `config.pb.go`
