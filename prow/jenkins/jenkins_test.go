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

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
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
			fmt.Fprint(w, `{"items": [
				{
					"actions": [
						{
							"parameters": [
								{
									"name": "BUILD_ID",
									"value": "queued-int"
								},
								{
									"name": "PROW_JOB_ID",
									"value": "queued_pj_id"
								}
							]
						}
					],
					"task": {
						"name": "PR-763"
					}
				}
			]}`)
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
		fmt.Fprintf(w, `{"builds": %s}`, string(data))
	}
}

func strP(str string) *string {
	return &str
}

func intP(i int) *int {
	return &i
}

func TestListBuilds(t *testing.T) {
	type Task struct {
		// Used for tracking unscheduled builds for jobs.
		Name string `json:"name"`
	}

	tests := []struct {
		name string

		existingJobs  []string
		requestedJobs []BuildQueryParams
		builds        map[string][]Build
		status        *int

		expectedResults map[string]Build
		expectedErr     error
	}{
		{
			name: "missing job does not block",

			existingJobs:  []string{"unit", "integration"},
			requestedJobs: []BuildQueryParams{{JobName: "unit", ProwJobID: "unitpj"}, {JobName: "unit", ProwJobID: "queued_pj_id"}, {JobName: "integration", ProwJobID: "integrationpj"}, {JobName: "e2e", ProwJobID: "e2epj"}},
			builds: map[string][]Build{
				"unit": {
					{Number: 1, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first"}, {Name: prowJobID, Value: "first"}}}}},
					{Number: 2, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second"}, {Name: prowJobID, Value: "second"}}}}},
					{Number: 3, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "third"}, {Name: prowJobID, Value: "third"}}}}},
					{Number: 4, Result: strP(unstable), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "fourth"}, {Name: prowJobID, Value: "fourth"}}}}},
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
				"fourth":     {Number: 4, Result: strP(unstable), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "fourth"}, {Name: prowJobID, Value: "fourth"}}}}},
				"first-int":  {Number: 1, Result: strP(failure), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "first-int"}, {Name: prowJobID, Value: "first-int"}}}}},
				"second-int": {Number: 2, Result: strP(success), Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "second-int"}, {Name: prowJobID, Value: "second-int"}}}}},
				// queued_pj_id is returned from the testWrapper
				"queued_pj_id": {Number: 0, Result: nil, Actions: []Action{{Parameters: []Parameter{{Name: statusBuildID, Value: "queued-int"}, {Name: prowJobID, Value: "queued_pj_id"}}}}, enqueued: true, Task: Task{Name: "PR-763"}},
			},
		},
		{
			name: "bad error",

			existingJobs:  []string{"unit"},
			requestedJobs: []BuildQueryParams{{JobName: "unit", ProwJobID: "prowjobidhere"}},
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

func TestBuildCreate(t *testing.T) {
	testCases := []struct {
		name        string
		input       *prowapi.ProwJobSpec
		expectError bool
		statusCode  int
		output      []string
	}{
		{
			name: "GitHub Branch Source based PR job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			statusCode: 201,
			output: []string{
				"/job/my-jenkins-job-name/view/change-requests/job/PR-123/api/json",
				"/job/my-jenkins-job-name/view/change-requests/job/PR-123/build",
				"/job/my-jenkins-job-name/view/change-requests/job/PR-123/api/json",
				"/job/my-jenkins-job-name/view/change-requests/job/PR-123/5/stop",
				"/job/my-jenkins-job-name/view/change-requests/job/PR-123/buildWithParameters",
			},
		},
		{
			name: "GitHub Branch Source based branch job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Type:  prowapi.PostsubmitJob,
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
			},
			statusCode: 201,
			output: []string{
				"/job/my-jenkins-job-name/job/master/api/json",
				"/job/my-jenkins-job-name/job/master/build",
				"/job/my-jenkins-job-name/job/master/api/json",
				"/job/my-jenkins-job-name/job/master/5/stop",
				"/job/my-jenkins-job-name/job/master/buildWithParameters",
			},
		},
		{
			name: "Static Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			statusCode: 201,
			output: []string{
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/build",
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/5/stop",
				"/job/my-k8s-job-name/buildWithParameters",
			},
		},
		{
			name: "Non-404 error getting Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			statusCode:  500,
			expectError: true,
			// 5 times because Client.Get does retries on 5xx status code
			output: []string{
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/api/json",
				"/job/my-k8s-job-name/api/json",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualPaths := []string{}

			var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
				actualPaths = append(actualPaths, r.URL.Path)
				var response string

				if len(actualPaths) < 3 {
					response = `{"builds":[],"lastBuild":null,"property":[]}`
				} else {
					response = `{"builds":[{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":5,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/5/"},{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":4,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/4/"},{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":3,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/3/"},{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":2,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/2/"},{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":1,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/1/"}],"lastBuild":{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowRun","number":5,"url":"https://myjenkins.com/job/very-good-job/view/change-requests/job/PR-23/5/"},"property":[{"_class":"hudson.model.ParametersDefinitionProperty","parameterDefinitions":[{"_class":"hudson.model.StringParameterDefinition","defaultParameterValue":{"_class":"hudson.model.StringParameterValue","name":"PROW_JOB_ID","value":""},"description":"Prow Job ID – set when the job is triggered by Prow","name":"PROW_JOB_ID","type":"StringParameterDefinition"}]},{"_class":"org.jenkinsci.plugins.workflow.multibranch.BranchJobProperty","branch":{}}]}`
				}

				w.WriteHeader(testCase.statusCode)
				w.Write([]byte(response))
			}

			ts := httptest.NewServer(handler)
			defer ts.Close()

			jc := Client{
				logger:  logrus.WithField("client", "jenkins"),
				client:  ts.Client(),
				baseURL: ts.URL,
			}

			buildErr := jc.BuildFromSpec(testCase.input, "buildID", "prowJobID")

			if buildErr != nil && !testCase.expectError {
				t.Errorf("%s: unexpected build error: %v", testCase.name, buildErr)
			}

			if !reflect.DeepEqual(testCase.output, actualPaths) {
				t.Errorf("%s: expected path %s, got %s", testCase.name, testCase.output, actualPaths)
			}
		})
	}
}

func TestGetJobName(t *testing.T) {
	testCases := []struct {
		name   string
		input  *prowapi.ProwJobSpec
		output string
	}{
		{
			name: "GitHub Branch Source based PR job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "my-jenkins-job-name/view/change-requests/job/PR-123",
		},
		{
			name: "GitHub Branch Source based branch job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Type:  prowapi.PostsubmitJob,
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
			},
			output: "my-jenkins-job-name/job/master",
		},
		{
			name: "Static Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "my-k8s-job-name",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualValue := getJobName(testCase.input)

			if !reflect.DeepEqual(testCase.output, actualValue) {
				t.Errorf("%s: expected values %s, got %s", testCase.name, testCase.output, actualValue)
			}

		})
	}
}

func TestGetJobInfoPath(t *testing.T) {
	testCases := []struct {
		name   string
		input  *prowapi.ProwJobSpec
		output string
	}{
		{
			name: "GitHub Branch Source based PR job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-jenkins-job-name/view/change-requests/job/PR-123/api/json",
		},
		{
			name: "GitHub Branch Source based branch job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Type:  prowapi.PostsubmitJob,
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
			},
			output: "/job/my-jenkins-job-name/job/master/api/json",
		},
		{
			name: "Static Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-k8s-job-name/api/json",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualValue := getJobInfoPath(testCase.input)

			if !reflect.DeepEqual(testCase.output, actualValue) {
				t.Errorf("%s: expected values %s, got %s", testCase.name, testCase.output, actualValue)
			}

		})
	}
}

func TestGetBuildPath(t *testing.T) {
	testCases := []struct {
		name   string
		input  *prowapi.ProwJobSpec
		output string
	}{
		{
			name: "GitHub Branch Source based PR job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-jenkins-job-name/view/change-requests/job/PR-123/build",
		},
		{
			name: "GitHub Branch Source based branch job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Type:  prowapi.PostsubmitJob,
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
			},
			output: "/job/my-jenkins-job-name/job/master/build",
		},
		{
			name: "Static Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-k8s-job-name/build",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualValue := getBuildPath(testCase.input)

			if !reflect.DeepEqual(testCase.output, actualValue) {
				t.Errorf("%s: expected values %s, got %s", testCase.name, testCase.output, actualValue)
			}

		})
	}
}

func TestGetBuildWithParametersPath(t *testing.T) {
	testCases := []struct {
		name   string
		input  *prowapi.ProwJobSpec
		output string
	}{
		{
			name: "GitHub Branch Source based PR job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-jenkins-job-name/view/change-requests/job/PR-123/buildWithParameters",
		},
		{
			name: "GitHub Branch Source based branch job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Type:  prowapi.PostsubmitJob,
				Job:   "my-jenkins-job-name",
				JenkinsSpec: &prowapi.JenkinsSpec{
					GitHubBranchSourceJob: true,
				},
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
			},
			output: "/job/my-jenkins-job-name/job/master/buildWithParameters",
		},
		{
			name: "Static Jenkins job",
			input: &prowapi.ProwJobSpec{
				Agent: "jenkins",
				Job:   "my-k8s-job-name",
				Refs: &prowapi.Refs{
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			output: "/job/my-k8s-job-name/buildWithParameters",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualValue := getBuildWithParametersPath(testCase.input)

			if !reflect.DeepEqual(testCase.output, actualValue) {
				t.Errorf("%s: expected values %s, got %s", testCase.name, testCase.output, actualValue)
			}

		})
	}
}
