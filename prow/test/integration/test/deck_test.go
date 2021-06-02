/*
Copyright 2021 The Kubernetes Authors.

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

package integration

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestDeck(t *testing.T) {
	t.Parallel()

	resp, err := http.Get("http://localhost/deck")
	if err != nil {
		t.Fatalf("Failed getting deck front end %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Got status code %d, expected 200", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed getting deck body response content %v", err)
	}
	if !strings.Contains(string(body), "<title>Prow Status</title>") {
		t.Fatalf("Expected content not found in body %s", body)
	}
}
