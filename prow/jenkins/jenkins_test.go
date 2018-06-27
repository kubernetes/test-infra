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

package jenkins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// getRequestedJob is attempting to determine if this is a job-specific
// request in a pretty hacky way.
func getRequestedJob(path string) string {
	parts := strings.Split(path, "/")
	jobIndex := -1
	for i, part := range parts {
		if part == "job" {
			// This is a job-specific request. Record the index.
			jobIndex = i + 1
			break
		}
	}
	// If this is not a job-specific request, fail for now. Eventually we
	// are going to proxy queue requests.
	if jobIndex == -1 {
		return ""
	}
	// Sanity check
	if jobIndex+1 > len(parts) {
		return ""
	}
	return parts[jobIndex]
}

func testWrapper(t *testing.T, jobs []string, builds map[string][]Build, status *int) http.HandlerFunc {
	var paths []string
	for _, job := range jobs {
		paths = append(paths, fmt.Sprintf("/job/%s/api/json", job))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		t.Logf("test request to %q", r.URL.Path)

		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
			return
		}
		if r.URL.Path == "/queue/api/json" {
			fmt.Fprint(w, `{"items": []}`)
			return
		}
		var found bool
		for _, path := range paths {
			if r.URL.Path == path {
				found = true
			}
		}
		if !found {
			w.WriteHeader(404)
			return
		}
		if status != nil {
			w.WriteHeader(*status)
			return
		}
		requestedJob := getRequestedJob(r.URL.Path)
		data, err := json.Marshal(builds[requestedJob])
		if err != nil {
			t.Errorf("unexpected error while marshaling builds: %v", err)
			return
		}
		fmt.Fprint(w, fmt.Sprintf(`{"builds": %s}`, string(data)))
	}
}

func strP(str string) *string {
	return &str
}

func intP(i int) *int {
	return &i
}

func TestListBuilds(t *testing.T) {
	tests := []struct {
		name string

		existingJobs  []string
		requestedJobs []string
		builds        map[string][]Build
		status        *int

		expectedResults map[string]Build
		expectedErr     error
	}{
		{
			name: "missing job does not block",

			existingJobs:  []string{"unit", "integration"},
			requestedJobs: []string{"unit", "integration", "e2e"},
			builds: map[string][]Build{
				"unit": {
					{Number: 1, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first"}, {Name: prowJobID, Value: "first"}}}}},
					{Number: 2, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second"}, {Name: prowJobID, Value: "second"}}}}},
					{Number: 3, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "third"}, {Name: prowJobID, Value: "third"}}}}},
				},
				"integration": {
					{Number: 1, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first-int"}, {Name: prowJobID, Value: "first-int"}}}}},
					{Number: 2, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second-int"}, {Name: prowJobID, Value: "second-int"}}}}},
				},
			},

			expectedResults: map[string]Build{
				"first":      {Number: 1, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first"}, {Name: prowJobID, Value: "first"}}}}},
				"second":     {Number: 2, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second"}, {Name: prowJobID, Value: "second"}}}}},
				"third":      {Number: 3, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "third"}, {Name: prowJobID, Value: "third"}}}}},
				"first-int":  {Number: 1, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first-int"}, {Name: prowJobID, Value: "first-int"}}}}},
				"second-int": {Number: 2, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second-int"}, {Name: prowJobID, Value: "second-int"}}}}},
			},
		},
		{
			name: "bad error",

			existingJobs:  []string{"unit"},
			requestedJobs: []string{"unit"},
			status:        intP(502),

			expectedErr: fmt.Errorf("cannot list builds for job \"unit\": response not 2XX: 502 Bad Gateway"),
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)

		ts := httptest.NewServer(testWrapper(t, test.existingJobs, test.builds, test.status))
		defer ts.Close()

		jc := Client{
			logger:  logrus.WithField("client", "jenkins"),
			client:  ts.Client(),
			baseURL: ts.URL,
		}

		builds, err := jc.ListBuilds(test.requestedJobs)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("unexpected error: %v, expected: %v", err, test.expectedErr)
		}
		for expectedJob, expectedBuild := range test.expectedResults {
			gotBuild, exists := builds[expectedJob]
			if !exists {
				t.Errorf("expected job %q, got %v", expectedJob, builds)
				continue
			}
			if !reflect.DeepEqual(expectedBuild, gotBuild) {
				t.Errorf("expected build:\n%+v\ngot:\n%+v", expectedBuild, gotBuild)
			}
		}
	}
}
