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

func TestDeckTenanted(t *testing.T) {
	t.Parallel()

	resp, err := http.Get("http://localhost/deck-tenanted")
	if err != nil {
		t.Fatalf("Failed getting deck-tenanted front end %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected response status code %d, got %d, ", http.StatusOK, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed getting deck body response content %v", err)
	}
	if got, want := string(body), "<title>Prow Status</title>"; !strings.Contains(got, want) {
		firstLines := strings.Join(strings.SplitN(strings.TrimSpace(got), "\n", 30), "\n")
		t.Fatalf("Expected content %q not found in body %s [......]", want, firstLines)
	}
}
