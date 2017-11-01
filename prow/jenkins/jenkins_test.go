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

func testWrapper(t *testing.T, jobs []string, builds map[string][]JenkinsBuild, status *int) http.HandlerFunc {
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

func TestListJenkinsBuilds(t *testing.T) {
	tests := []struct {
		name string

		existingJobs  []string
		requestedJobs map[string]struct{}
		builds        map[string][]JenkinsBuild
		status        *int

		expectedResults map[string]JenkinsBuild
		expectedErr     error
	}{
		{
			name: "missing job does not block",

			existingJobs:  []string{"unit", "integration"},
			requestedJobs: map[string]struct{}{"unit": {}, "integration": {}, "e2e": {}},
			builds: map[string][]JenkinsBuild{
				"unit": {
					{Number: 1, Result: strP(Succeess), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "first"}}}}},
					{Number: 2, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "second"}}}}},
					{Number: 3, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "third"}}}}},
				},
				"integration": {
					{Number: 1, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "first-int"}}}}},
					{Number: 2, Result: strP(Succeess), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "second-int"}}}}},
				},
			},

			expectedResults: map[string]JenkinsBuild{
				"first":      {Number: 1, Result: strP(Succeess), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "first"}}}}},
				"second":     {Number: 2, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "second"}}}}},
				"third":      {Number: 3, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "third"}}}}},
				"first-int":  {Number: 1, Result: strP(Failure), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "first-int"}}}}},
				"second-int": {Number: 2, Result: strP(Succeess), Actions: []Action{{Parameters: []Parameter{{Name: buildID, Value: "second-int"}}}}},
			},
		},
		{
			name: "bad error",

			existingJobs:  []string{"unit"},
			requestedJobs: map[string]struct{}{"unit": {}},
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

		builds, err := jc.ListJenkinsBuilds(test.requestedJobs)
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
