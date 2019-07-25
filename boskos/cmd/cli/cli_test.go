/*
Copyright 2019 The Kubernetes Authors.

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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

type request struct {
	method string
	header http.Header
	url    url.URL
	body   []byte
}

type response struct {
	code int
	data []byte
}

func TestCommand(t *testing.T) {
	var testCases = []struct {
		name           string
		args           []string
		responses      map[string]response
		expectedCalls  []request
		expectedErr    bool // this means cobra error, print usage
		expectedCode   int  // this is the real exit code
		expectedOutput string
	}{
		{
			name:        "no flags fails",
			args:        []string{"acquire"},
			expectedErr: true,
			expectedOutput: `Error: required flag(s) "state", "target-state", "type" not set
Usage:
  boskosctl acquire [flags]

Flags:
  -h, --help                  help for acquire
      --request-id string     request id to acquire the resource in
      --state string          State to acquire the resource in
      --target-state string   Move resource to this state after acquiring
      --timeout duration      If set, retry this long until the resource has been acquired
      --type string           Type of resource to acquire

Global Flags:
      --owner-name string   Name identifying the user of this client
      --server-url string   URL of the Boskos server

`,
		},
		{
			name: "normal acquire sends a request and succeeds",
			args: []string{"acquire", "--state=new", "--type=thing", "--target-state=old"},
			responses: map[string]response{
				"/acquire": {
					code: http.StatusOK,
					data: []byte(`{"type":"thing","name":"87527b0c-eac2-4f83-9a03-791b2239e093","state":"old","owner":"test","lastupdate":"2019-07-24T23:30:40.094116858Z","userdata":{}}`),
				},
			},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/acquire", RawQuery: `dest=old&owner=test&state=new&type=thing`},
				body:   []byte{},
			}},
			expectedOutput: `{"type":"thing","name":"87527b0c-eac2-4f83-9a03-791b2239e093","state":"old","owner":"test","lastupdate":"2019-07-24T23:30:40.094116858Z","userdata":{}}
`,
		},
		{
			name: "normal acquire sends a request and fails on bad response",
			args: []string{"acquire", "--state=new", "--type=thing", "--target-state=old"},
			responses: map[string]response{
				"/acquire": {
					code: http.StatusOK,
					data: []byte(`nonsense`),
				},
			},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/acquire", RawQuery: `dest=old&owner=test&state=new&type=thing`},
				body:   []byte{},
			}},
			expectedOutput: `failed to acquire a resource: invalid character 'o' in literal null (expecting 'u')
`,
			expectedCode: 1,
		},
		{
			name:      "normal acquire sends a request and fails on 404",
			args:      []string{"acquire", "--state=new", "--type=thing", "--target-state=old"},
			responses: map[string]response{},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/acquire", RawQuery: `dest=old&owner=test&state=new&type=thing`},
				body:   []byte{},
			}},
			expectedCode: 1,
			expectedOutput: `failed to acquire a resource: resources not found
`,
		},
		{
			name: "retry acquire sends requests and times out",
			args: []string{"acquire", "--state=new", "--type=thing", "--target-state=old", "--timeout=5s", "--request-id=testingRequest"},
			responses: map[string]response{
				"/acquire": {
					code: http.StatusUnauthorized,
				},
			},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/acquire", RawQuery: `dest=old&owner=test&state=new&type=thing&request_id=testingRequest`},
				body:   []byte{},
			}, {
				method: http.MethodPost,
				url:    url.URL{Path: "/acquire", RawQuery: `dest=old&owner=test&state=new&type=thing&request_id=testingRequest`},
				body:   []byte{},
			}},
			expectedOutput: `failed to acquire a resource: resources already used by another user
`,
			expectedCode: 1,
		},
		{
			name: "normal release sends a request and succeeds",
			args: []string{"release", "--name=identifier", "--target-state=old"},
			responses: map[string]response{
				"/release": {
					code: http.StatusOK,
				},
			},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/release", RawQuery: `dest=old&owner=test&name=identifier`},
				body:   []byte{},
			}},
			expectedOutput: `released resource "identifier"
`,
		},
		{
			name:        "normal release without flags fails",
			args:        []string{"release"},
			expectedErr: true,
			expectedOutput: `Error: required flag(s) "name", "target-state" not set
Usage:
  boskosctl release [flags]

Flags:
  -h, --help                  help for release
      --name string           Name of the resource lease to release
      --target-state string   Move resource to this state after releasing

Global Flags:
      --owner-name string   Name identifying the user of this client
      --server-url string   URL of the Boskos server

`,
		},
		{
			name: "failed release sends a request and fails",
			args: []string{"release", "--name=identifier", "--target-state=old"},
			responses: map[string]response{
				"/release": {
					code: http.StatusNotFound,
				},
			},
			expectedCalls: []request{{
				method: http.MethodPost,
				url:    url.URL{Path: "/release", RawQuery: `dest=old&owner=test&name=identifier`},
				body:   []byte{},
			}},
			expectedCode: 1,
			expectedOutput: `failed to release resource "identifier": status 404 Not Found, statusCode 404 releasing identifier
`,
		},
	}

	for _, testCase := range testCases {
		var recordedCalls []request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read request body in test server: %v", err)
			}
			recordedCalls = append(recordedCalls, request{
				method: r.Method,
				header: r.Header,
				url:    *r.URL,
				body:   body,
			})
			for path, response := range testCase.responses {
				if r.URL.Path == path {
					w.WriteHeader(response.code)
					if _, err := w.Write(response.data); err != nil {
						t.Fatalf("failed to write response in test server: %v", err)
					}
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}))

		var exitCode int
		exit = func(i int) {
			exitCode = i
		}

		cmd := command()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs(append(testCase.args, fmt.Sprintf("--server-url=%s", server.URL), "--owner-name=test"))
		err := cmd.Execute()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}

		if expected, actual := testCase.expectedOutput, buf.String(); expected != actual {
			t.Errorf("%s: got incorrect output: %s", testCase.name, diff.StringDiff(expected, actual))
		}

		if expected, actual := len(testCase.expectedCalls), len(recordedCalls); expected != actual {
			t.Errorf("%s: expected %d calls to boskos but saw %d", testCase.name, expected, actual)
		}

		if expected, actual := testCase.expectedCode, exitCode; expected != actual {
			t.Errorf("%s: expected to exit with %d, but saw %d", testCase.name, expected, actual)
		}

		for i, request := range recordedCalls {
			if expected, actual := testCase.expectedCalls[i].method, request.method; expected != actual {
				t.Errorf("%s: request %d: incorrect method, expected %s, saw %s", testCase.name, i, expected, actual)
			}

			if expected, actual := testCase.expectedCalls[i].url.Path, request.url.Path; expected != actual {
				t.Errorf("%s: request %d: incorrect path, expected %s, saw %s", testCase.name, i, expected, actual)
			}

			if expected, actual := testCase.expectedCalls[i].url.Query(), request.url.Query(); !reflect.DeepEqual(expected, actual) {
				t.Errorf("%s: request %d: incorrect query: %s", testCase.name, i, diff.ObjectReflectDiff(expected, actual))
			}

			if expected, actual := testCase.expectedCalls[i].header.Get("Content-Type"), request.header.Get("Content-Type"); expected != actual {
				t.Errorf("%s: request %d: incorrect content-type header, expected %s, saw %s", testCase.name, i, expected, actual)
			}

			if expected, actual := testCase.expectedCalls[i].body, request.body; !reflect.DeepEqual(expected, actual) {
				t.Errorf("%s: request %d: incorrect body: %s", testCase.name, i, diff.StringDiff(string(expected), string(actual)))
			}
		}
	}
}
