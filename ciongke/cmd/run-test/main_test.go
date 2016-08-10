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

package main

import (
	"io/ioutil"
	"testing"

	"github.com/kubernetes/test-infra/ciongke/gcs/fakegcs"
)

func TestGetSource(t *testing.T) {
	gc := fakegcs.FakeClient{}
	gc.Objects = make(map[string]map[string][]byte)
	gc.Objects["sb"] = make(map[string][]byte)
	gc.Objects["sb"]["5.tar.gz"] = []byte("source")
	tc := testClient{
		PRNumber:     5,
		SourceBucket: "sb",
		GCSClient:    &gc,
	}
	r, err := tc.getSource()
	if err != nil {
		t.Fatalf("Didn't expect error getting source: %s", err)
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("Error reading downloaded source: %s", err)
	}
	if string(b) != "source" {
		t.Fatalf("Expected \"source\", got \"%s\"", string(b))
	}
}
