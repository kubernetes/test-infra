/*
Copyright 2020 The Kubernetes Authors.

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

package main

import (
	"strings"
	"testing"
)

const validURL = "http://example.org"

func makeOptionsForDescriptionTest(descriptionURL string) options {
	return options{descriptionURL: descriptionURL, org: "exampleOrg", repo: "exampleRepo", dryRun: true, retireContext: "retireContext"}
}

func TestDescriptionContainsNil(t *testing.T) {
	o := options{org: "exampleOrg", repo: "exampleRepo", dryRun: true, retireContext: "retireContext"}
	err := o.Validate()
	if err != nil {
		t.Errorf("No error expected for description nil, got %v", err)
	}
}

func TestDescriptionContainsURL(t *testing.T) {
	o := makeOptionsForDescriptionTest(validURL)
	err := o.Validate()
	if err != nil {
		t.Errorf("No error expected for description %s, got %v", validURL, err)
	}
}

func TestDescriptionContainsNothing(t *testing.T) {
	o := makeOptionsForDescriptionTest("")
	err := o.Validate()
	if err != nil {
		t.Errorf("No error expected for description %s, got %v", "", err)
	}
}

func TestDescriptionDoesNotContainAURL(t *testing.T) {
	o := makeOptionsForDescriptionTest("test")
	err := o.Validate()
	if err == nil {
		t.Errorf("Error expected, got nil")
	}
	if !strings.Contains(err.Error(), "'test'") {
		t.Errorf("Error expected to contain wrong url, got %s", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid URI") {
		t.Errorf("Error expected to contain parse error description, got %s", err.Error())
	}
}
