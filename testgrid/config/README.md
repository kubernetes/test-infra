# Opensource config for testgrid.k8s.io

build : 
  go build ./yaml2proto
  go install

usage:
config \<input/path/to/yaml\> \<output/path/to/proto\>

# 
User should only add/update config.yaml
----------------------------------------------------------------------------------------
-- Yaml representation for configuring testgrid.k8s.io

To add a new test:

1. Append a new testgroup under test_groups, specity the name and where to get the log.

2. Append a new dashboardtab under the dashboard you would like to add the testgroup to,

  or create a new dashboard and assign the testgroup to the dashboard.
  
  * The testgroup name from a dashboardtab should match the name from a testgroup
  
  ** Note that a testgroup can be within multiple dashboards. 
  
3. Run `config_test.go` to make sure the config is valid.

You can also add notifications to a testgroup (which displays on any dashboardtab backed

by that testgroup) or dashboard (which displays on each tab in that dashboard).

To add a notification:

1. Append 'notifications' under the testgroup or dashboard.

  * Each notification has a required 'summary' field (short text description of the 
  
  notice or issue), and an optional 'context_link' field (fully-qualified URL
  
  to more information).

2. Run `config_test.go` to make sure the config is valid.

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


Contact us before you make changes to config.proto

#
Devs -
If you changed config.proto, do:

1. Make sure [protoc](https://github.com/golang/protobuf) is installed and

2. `protoc --go_out=pb config.proto`

3. Please kindly search-replace all `json:"foo,omitempty"` with `yaml:"foo,omitempty"`.

   -- replace all `json:"` with `yaml:"` would work
   
4. Commit both config.proto and config.pb.go
