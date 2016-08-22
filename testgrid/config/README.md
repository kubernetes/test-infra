# opensource testgrid config

#### TODO - 
#### 1. put into k8s automatically
#### 2. column_header field

build : 
  build yaml2proto pkg
  build config main.go

usage:
config \<input/path/to/yaml\> \<output/path/to/proto\>

----------------------------------------------------------------------------------------
/
- config.yaml      : open source config file in YAML, edit the YAML file as you need
- config.proto     : class structure of a valid testgrid configuration, for reference
/pb
- config.pb.go     : compiled go object of config.proto, yaml friendly
/yaml2proto
  - /testdata      : yaml files for testing
  - yaml2proto.go  : yaml2proto method implementation
  
----------------------------------------------------------------------------------------
