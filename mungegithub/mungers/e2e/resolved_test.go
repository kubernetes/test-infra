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

package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolutionTracker(t *testing.T) {
	m := http.NewServeMux()
	rt := NewResolutionTracker()
	m.HandleFunc("/get", rt.GetHTTP)
	m.HandleFunc("/set", rt.SetHTTP)
	s := httptest.NewServer(m)
	defer s.Close()

	// Mark resolved
	resp, err := http.Get(s.URL + "/set?job=aoeu&number=64")
	if err != nil {
		t.Errorf("fail: %v", err)
	} else {
		resp.Body.Close()
	}
	if e, a := true, rt.Resolved("aoeu", 64); e != a {
		t.Errorf("Expected %v got %v", e, a)
	}

	// Mark unresolved
	resp, err = http.Get(s.URL + "/set?job=aoeu&number=64&resolved=false")
	if err != nil {
		t.Errorf("fail: %v", err)
	} else {
		resp.Body.Close()
	}
	if e, a := false, rt.Resolved("aoeu", 64); e != a {
		t.Errorf("Expected %v got %v", e, a)
	}
}
