/*
Copyright 2016 The Kubernetes Authors.

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

	"github.com/golang/protobuf/proto"
	pb "k8s.io/test-infra/testgrid/config/pb"
)

func TestYaml2ProtoEmpty(t *testing.T) {
	_, err := Yaml2Proto([]byte(""))
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Testgroups" {
		t.Errorf("Should Throw No Valid Testgroups Error\n")
	}
}

func TestYaml2ProtoNoTestgroup(t *testing.T) {
	yaml  :=
`dashboards:
- name: dashboard_1`

	_, err := Yaml2Proto([]byte(yaml))
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Testgroups" {
		t.Errorf("Should Throw No Valid Testgroups Error\n")
	}
}

func TestYaml2ProtoNoDashboard(t *testing.T) {
	yaml :=   
`test_groups:
- name: testgroup_1`

	_, err := Yaml2Proto([]byte(yaml))
	
	if err == nil || err.Error() != "Invalid Yaml : No Valid Dashboards" {
		t.Errorf("Should Throw No Valid Dashboards Error\n")
	}
}

func TestYaml2ProtoIsExternalAndUseKuberClientFalse(t *testing.T) {
	yaml :=
`default_test_group:
  name: default
default_dashboard_tab:
  name: default
test_groups:
- name: testgroup_1
dashboards:
- name: dashboard_1`

	protobufData, err := Yaml2Proto([]byte(yaml))
	
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
		
		if !testgroup.UseKubernetesClient {
			t.Errorf("UseKubernetesClient should always be true!")
		}
	}
}

func TestYaml2ProtoNoDefaultTestgroup(t *testing.T) {
	yaml :=
`default_dashboard_tab:
test_groups:
- name: testgroup_1
dashboards:
- name: dashboard_1`

	_, err := Yaml2Proto([]byte(yaml))
	
	if err == nil || err.Error() != "Please Include The Default Testgroup & Dashboardtab" {
		t.Errorf("Should Throw No Default Testgroup Error\n")
	}
}
