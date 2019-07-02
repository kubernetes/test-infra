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

package bugzilla

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
)

var (
	bugData   = []byte(`{"bugs":[{"alias":[],"assigned_to":"Steve Kuznetsov","assigned_to_detail":{"email":"skuznets","id":381851,"name":"skuznets","real_name":"Steve Kuznetsov"},"blocks":[],"cc":["Sudha Ponnaganti"],"cc_detail":[{"email":"sponnaga","id":426940,"name":"sponnaga","real_name":"Sudha Ponnaganti"}],"classification":"Red Hat","component":["Test Infrastructure"],"creation_time":"2019-05-01T19:33:36Z","creator":"Dan Mace","creator_detail":{"email":"dmace","id":330250,"name":"dmace","real_name":"Dan Mace"},"deadline":null,"depends_on":[],"docs_contact":"","dupe_of":null,"groups":[],"id":1705243,"is_cc_accessible":true,"is_confirmed":true,"is_creator_accessible":true,"is_open":true,"keywords":[],"last_change_time":"2019-05-17T15:13:13Z","op_sys":"Unspecified","platform":"Unspecified","priority":"unspecified","product":"OpenShift Container Platform","qa_contact":"","resolution":"","see_also":[],"severity":"medium","status":"VERIFIED","summary":"[ci] cli image flake affecting *-images jobs","target_milestone":"---","target_release":["3.11.z"],"url":"","version":["3.11.0"],"whiteboard":""}],"faults":[]}`)
	bugStruct = &Bug{Alias: []int{}, AssignedTo: "Steve Kuznetsov", AssignedToDetail: &User{Email: "skuznets", ID: 381851, Name: "skuznets", RealName: "Steve Kuznetsov"}, Blocks: []int{}, CC: []string{"Sudha Ponnaganti"}, CCDetail: []User{{Email: "sponnaga", ID: 426940, Name: "sponnaga", RealName: "Sudha Ponnaganti"}}, Classification: "Red Hat", Component: []string{"Test Infrastructure"}, CreationTime: "2019-05-01T19:33:36Z", Creator: "Dan Mace", CreatorDetail: &User{Email: "dmace", ID: 330250, Name: "dmace", RealName: "Dan Mace"}, DependsOn: []int{}, ID: 1705243, IsCCAccessible: true, IsConfirmed: true, IsCreatorAccessible: true, IsOpen: true, Groups: []string{}, Keywords: []string{}, LastChangeTime: "2019-05-17T15:13:13Z", OperatingSystem: "Unspecified", Platform: "Unspecified", Priority: "unspecified", Product: "OpenShift Container Platform", SeeAlso: []string{}, Severity: "medium", Status: "VERIFIED", Summary: "[ci] cli image flake affecting *-images jobs", TargetRelease: []string{"3.11.z"}, TargetMilestone: "---", Version: []string{"3.11.0"}}
)

func clientForUrl(url string) *client {
	return &client{
		logger:   logrus.WithField("testing", "true"),
		endpoint: url,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		getAPIKey: func() []byte {
			return []byte("api-key")
		},
	}
}

func TestGetBug(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-BUGZILLA-API-KEY") != "api-key" {
			t.Error("did not get api-key passed in X-BUGZILLA-API-KEY header")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("api_key") != "api-key" {
			t.Error("did not get api-key passed in api_key query parameter")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("incorrect method to get a bug: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/bug/") {
			t.Errorf("incorrect path to get a bug: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/rest/bug/")); err != nil {
			t.Errorf("malformed bug id: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		} else {
			if id == 1705243 {
				w.Write(bugData)
			} else {
				http.Error(w, "404 Not Found", http.StatusNotFound)
			}
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	// this should give us what we want
	bug, err := client.GetBug(1705243)
	if err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	}
	if !reflect.DeepEqual(bug, bugStruct) {
		t.Errorf("got incorrect bug: %v", diff.ObjectReflectDiff(bug, bugStruct))
	}

	// this should 404
	otherBug, err := client.GetBug(1)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}
	if otherBug != nil {
		t.Errorf("expected no bug, got: %v", otherBug)
	}
}

func TestUpdateBug(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-BUGZILLA-API-KEY") != "api-key" {
			t.Error("did not get api-key passed in X-BUGZILLA-API-KEY header")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("did not correctly set content-type header for JSON")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("api_key") != "api-key" {
			t.Error("did not get api-key passed in api_key query parameter")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPut {
			t.Errorf("incorrect method to update a bug: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/bug/") {
			t.Errorf("incorrect path to update a bug: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/rest/bug/")); err != nil {
			t.Errorf("malformed bug id: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		} else {
			if id == 1705243 {
				raw, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read update body: %v", err)
				}
				if actual, expected := string(raw), `{"status":"UPDATED"}`; actual != expected {
					t.Errorf("got incorrect udpate: expected %v, got %v", expected, actual)
				}
			} else {
				http.Error(w, "404 Not Found", http.StatusNotFound)
			}
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	// this should run an update
	if err := client.UpdateBug(1705243, BugUpdate{Status: "UPDATED"}); err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	}

	// this should 404
	err := client.UpdateBug(1, BugUpdate{Status: "UPDATE"})
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}
}

func TestAddPullRequestAsExternalBug(t *testing.T) {
	var testCases = []struct {
		name            string
		id              int
		expectedPayload string
		response        string
		expectedError   bool
		expectedChanged bool
	}{
		{
			name:            "update succeeds, makes a change",
			id:              1705243,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705243],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"bugs":[{"alias":[],"changes":{"ext_bz_bug_map.ext_bz_bug_id":{"added":"Github org/repo/pull/1","removed":""}},"id":1705243}]}}`,
			expectedError:   false,
			expectedChanged: true,
		},
		{
			name:            "update succeeds, makes no change",
			id:              1705244,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705244],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"bugs":[]}}`,
			expectedError:   false,
			expectedChanged: false,
		},
		{
			name:            "update fails, makes no change",
			id:              1705246,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705246],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":{"code": 100400,"message":"Invalid params for JSONRPC 1.0."},"id":"identifier","result":null}`,
			expectedError:   true,
			expectedChanged: false,
		},
		{
			name:            "get unrelated JSONRPC response",
			id:              1705247,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705247],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":null,"id":"oops","result":{"bugs":[]}}`,
			expectedError:   true,
			expectedChanged: false,
		},
	}
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("did not correctly set content-type header for JSON")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("incorrect method to use the JSONRPC API: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/jsonrpc.cgi" {
			t.Errorf("incorrect path to use the JSONRPC API: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		var payload struct {
			// Version is the version of JSONRPC to use. All Bugzilla servers
			// support 1.0. Some support 1.1 and some support 2.0
			Version string `json:"jsonrpc"`
			Method  string `json:"method"`
			// Parameters must be specified in JSONRPC 1.0 as a structure in the first
			// index of this slice
			Parameters []AddExternalBugParameters `json:"params"`
			ID         string                     `json:"id"`
		}
		raw, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			http.Error(w, "500 Server Error", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("malformed JSONRPC payload: %s", string(raw))
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		for _, testCase := range testCases {
			if payload.Parameters[0].BugIDs[0] == testCase.id {
				if actual, expected := string(raw), testCase.expectedPayload; actual != expected {
					t.Errorf("%s: got incorrect JSONRPC payload: %v", testCase.name, diff.ObjectReflectDiff(expected, actual))
				}
				if _, err := w.Write([]byte(testCase.response)); err != nil {
					t.Fatalf("%s: failed to send JSONRPC response: %v", testCase.name, err)
				}
				return
			}
		}
		http.Error(w, "404 Not Found", http.StatusNotFound)
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			changed, err := client.AddPullRequestAsExternalBug(testCase.id, "org", "repo", 1)
			if !testCase.expectedError && err != nil {
				t.Errorf("%s: expected no error, but got one: %v", testCase.name, err)
			}
			if testCase.expectedError && err == nil {
				t.Errorf("%s: expected an error, but got none", testCase.name)
			}
			if testCase.expectedChanged != changed {
				t.Errorf("%s: got incorrect state change", testCase.name)
			}
		})
	}

	// this should 404
	changed, err := client.AddPullRequestAsExternalBug(1, "org", "repo", 1)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}
	if changed {
		t.Error("expected not to change state, but did")
	}
}
