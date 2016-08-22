package yaml2proto

import (
	"testing"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	pb "k8s.io/test-infra/testgrid/config/pb"
)

func TestYaml2ProtoSmall(t *testing.T) {
	// convert a small yaml
	inFile := "testdata/small.yaml"
	outFile := "testdata/small.protobuf"

	err := Yaml2Proto(inFile,outFile)
	if ( err != nil ) {
		t.Errorf("Convert Error: %v\n", err)
	}

	in, err := ioutil.ReadFile(outFile)
	if err != nil {
		t.Errorf("Error reading file: %v\n", err)
	}
	config := &pb.Configuration{}
	if err := proto.Unmarshal(in, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	if ( len(config.TestGroups) != 1 ){
		t.Errorf("TestGroup Count Not One: %v\n", len(config.TestGroups))
	}

	if ( len(config.Dashboards) != 1 ){
		t.Errorf("Dashboard Count Not One: %v\n", len(config.Dashboards))
	}

	if ( *config.TestGroups[0].Name != "test_group_1" ){
		t.Errorf("TestGroup Name Not \"test_group_1\": %v\n", *config.TestGroups[0].Name)
	}
} 

func TestYaml2ProtoLarge(t *testing.T) {
	// convert a large yaml
	inFile := "testdata/large.yaml"
	outFile := "testdata/large.protobuf"

	err := Yaml2Proto(inFile,outFile)
	
	if ( err != nil ) {
		t.Errorf("Convert Error: %v\n", err)
	}

	in, err := ioutil.ReadFile(outFile)
	if err != nil {
		t.Errorf("Error reading file: %v\n", err)
	}
	config := &pb.Configuration{}
	if err := proto.Unmarshal(in, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	if ( len(config.TestGroups) != 167 ){
		t.Errorf("TestGroup Count Not 167: %v\n", len(config.TestGroups))
	}

	if ( len(config.Dashboards) != 14 ){
		t.Errorf("Dashboard Count Not 14: %v\n", len(config.Dashboards))
	}
}


