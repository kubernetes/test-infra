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


Contact us before you make changes to config.proto

#
Devs -
If you changed config.proto, do:

1. Make sure [protoc](https://github.com/golang/protobuf) is installed and

2. `protoc --go_out=pb config.proto`

3. Please kindly search-replace all `json:"foo,omitempty"` with `yaml:"foo,omitempty"`.

   -- replace all `json:"` with `yaml:"` would work
   
4. Commit both config.proto and config.pb.go
