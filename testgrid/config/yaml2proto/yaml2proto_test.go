/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package yaml2proto

import (
	"testing"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	pb "k8s.io/test-infra/testgrid/config/pb"
)

func TestYaml2ProtoEmpty(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/Empty.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File Empty.yaml\n")
	}

	_, err = Yaml2Proto(yamlData)
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Testgroups" {
		t.Errorf("Should Throw No Valid Testgroups Error\n")
	}
}

func TestYaml2ProtoNoTestgroup(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/NoTestgroup.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File NoTestgroup.yaml\n")
	}

	_, err = Yaml2Proto(yamlData)
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Testgroups" {
		t.Errorf("Should Throw No Valid Testgroups Error\n")
	}
}

func TestYaml2ProtoNoDashboard(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/NoDashboard.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File NoDashboard.yaml\n")
	}

	_, err = Yaml2Proto(yamlData)
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Dashboards" {
		t.Errorf("Should Throw No Valid Dashboards Error\n")
	}
}

func TestYaml2ProtoIsExternalFalse(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/IsExternalFalse.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File IsExternalFalse.yaml\n")
	}

	protobufData, err := Yaml2Proto(yamlData)
	
	if err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	config := &pb.Configuration{}
	if err := proto.Unmarshal(protobufData, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	for _,testgroup := range config.TestGroups {
		if !testgroup.IsExternal {
			t.Errorf("IsExternal should always be true!")
		}
	}
}

func TestYaml2ProtoUseKuberClientFalse(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/UseKuberClientFalse.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File UseKuberClientFalse.yaml\n")
	}

	protobufData, err := Yaml2Proto(yamlData)
	
	if err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	config := &pb.Configuration{}
	if err := proto.Unmarshal(protobufData, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	for _,testgroup := range config.TestGroups {
		if !testgroup.UseKubernetesClient {
			t.Errorf("UseKubernetesClient should always be true!")
		}
	}
}

func TestYaml2ProtoNoDefaultTestgroup(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/NoDefaultTestGroup.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File NoDefaultTestGroup.yaml\n")
	}

	_, err = Yaml2Proto(yamlData)
	
	if err == nil || err.Error() != "Please Include The Default Testgroup & Dashboardtab" {
		t.Errorf("Should Throw No Default Testgroup Error\n")
	}
}

func TestYaml2ProtoNoDefaultDashboardTab(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/NoDefaultDashboardTab.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File NoDefaultDashboardTab.yaml\n")
	}

	_, err = Yaml2Proto(yamlData)
	
	if err == nil || err.Error() != "Please Include The Default Testgroup & Dashboardtab" {
		t.Errorf("Should Throw No Default Dashboardtab Error\n")
	}
}

func TestYaml2ProtoLarge(t *testing.T) {
	yamlData, err := ioutil.ReadFile("testdata/large.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File large.yaml\n")
	}

	protobufData, err := Yaml2Proto(yamlData)
	
	if err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	config := &pb.Configuration{}
	if err := proto.Unmarshal(protobufData, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	if len(config.TestGroups) != 179 {
		t.Errorf("TestGroup Count Not 179: %v\n", len(config.TestGroups))
	}

	if config.TestGroups[0].DaysOfResults != 14 {
		t.Errorf("TestGroup DaysOfResults Not 14 By Default: %v\n", config.TestGroups[0].DaysOfResults)
	}

	if len(config.Dashboards) != 14 {
		t.Errorf("Dashboard Count Not 14: %v\n", len(config.Dashboards))
	}
}


