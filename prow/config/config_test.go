/*
Copyright 2017 The Kubernetes Authors.

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

package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestConfigLoads(t *testing.T) {
	_, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
}

func TestConfigSecurityJobsMatch(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	kp := c.Presubmits["kubernetes/kubernetes"]
	sp := c.Presubmits["kubernetes-security/kubernetes"]
	if len(kp) != len(sp) {
		t.Fatalf("length of kubernetes/kubernetes presumits %d does not equal length of kubernetes-security/kubernetes presubmits %d", len(kp), len(sp))
	}
	for i, j := range kp {
		name := strings.Replace(j.Name, "pull-kubernetes", "pull-security-kubernetes", -1)
		if name != sp[i].Name {
			t.Fatalf("%s should match %s", name, sp[i].Name)
		}
		j.Name = name
		j.RerunCommand = strings.Replace(j.RerunCommand, "pull-kubernetes", "pull-security-kubernetes", -1)
		j.Trigger = strings.Replace(j.Trigger, "pull-kubernetes", "pull-security-kubernetes", -1)
		j.Context = strings.Replace(j.Context, "pull-kubernetes", "pull-security-kubernetes", -1)
		j.re = sp[i].re
		if !reflect.DeepEqual(j, sp[i]) {
			t.Fatalf("kubernetes/kubernetes prow config jobs do not match kubernetes-security/kubernetes jobs:\n%#v\nshould match: %#v", j, sp[i])
		}
	}
}
