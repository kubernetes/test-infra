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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	bugData         = []byte(`{"bugs":[{"alias":[],"assigned_to":"Steve Kuznetsov","assigned_to_detail":{"email":"skuznets","id":381851,"name":"skuznets","real_name":"Steve Kuznetsov"},"blocks":[],"cc":["Sudha Ponnaganti"],"cc_detail":[{"email":"sponnaga","id":426940,"name":"sponnaga","real_name":"Sudha Ponnaganti"}],"classification":"Red Hat","component":["Test Infrastructure"],"creation_time":"2019-05-01T19:33:36Z","creator":"Dan Mace","creator_detail":{"email":"dmace","id":330250,"name":"dmace","real_name":"Dan Mace"},"deadline":null,"depends_on":[],"docs_contact":"","dupe_of":null,"groups":[],"id":1705243,"is_cc_accessible":true,"is_confirmed":true,"is_creator_accessible":true,"is_open":true,"keywords":[],"last_change_time":"2019-05-17T15:13:13Z","op_sys":"Unspecified","platform":"Unspecified","priority":"unspecified","product":"OpenShift Container Platform","qa_contact":"","resolution":"","see_also":[],"severity":"medium","status":"VERIFIED","summary":"[ci] cli image flake affecting *-images jobs","target_milestone":"---","target_release":["3.11.z"],"url":"","version":["3.11.0"],"whiteboard":""}],"faults":[]}`)
	bugStruct       = &Bug{Alias: []string{}, AssignedTo: "Steve Kuznetsov", AssignedToDetail: &User{Email: "skuznets", ID: 381851, Name: "skuznets", RealName: "Steve Kuznetsov"}, Blocks: []int{}, CC: []string{"Sudha Ponnaganti"}, CCDetail: []User{{Email: "sponnaga", ID: 426940, Name: "sponnaga", RealName: "Sudha Ponnaganti"}}, Classification: "Red Hat", Component: []string{"Test Infrastructure"}, CreationTime: "2019-05-01T19:33:36Z", Creator: "Dan Mace", CreatorDetail: &User{Email: "dmace", ID: 330250, Name: "dmace", RealName: "Dan Mace"}, DependsOn: []int{}, ID: 1705243, IsCCAccessible: true, IsConfirmed: true, IsCreatorAccessible: true, IsOpen: true, Groups: []string{}, Keywords: []string{}, LastChangeTime: "2019-05-17T15:13:13Z", OperatingSystem: "Unspecified", Platform: "Unspecified", Priority: "unspecified", Product: "OpenShift Container Platform", SeeAlso: []string{}, Severity: "medium", Status: "VERIFIED", Summary: "[ci] cli image flake affecting *-images jobs", TargetRelease: []string{"3.11.z"}, TargetMilestone: "---", Version: []string{"3.11.0"}}
	bugAccessDenied = []byte(`{"error":true,"code":102,"message":"You are not authorized to access bug #2. To see this bug, you must first log in to an account with the appropriate permissions."}`)
	bugInvalidBugID = []byte(`{"error":true,"code":101,"message":"Bug #3 does not exist."}`)
)

func clientForUrl(url string) *client {
	return &client{
		logger: logrus.WithField("testing", "true"),
		delegate: &delegate{
			endpoint: url,
			client: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
			getAPIKey: func() []byte {
				return []byte("api-key")
			},
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
			} else if id == 2 {
				w.Write(bugAccessDenied)
			} else if id == 3 {
				w.Write(bugInvalidBugID)
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
	if diff := cmp.Diff(bug, bugStruct); diff != "" {
		t.Errorf("got incorrect bug: %v", diff)
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

	// this should return access denied
	accessDeniedBug, err := client.GetBug(2)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsAccessDenied(err) {
		t.Errorf("expected an access denied error, got %v", err)
	}
	if accessDeniedBug != nil {
		t.Errorf("expected no bug, got: %v", accessDeniedBug)
	}

	// this should return invalid Bug ID
	invalidIDBug, err := client.GetBug(3)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsInvalidBugID(err) {
		t.Errorf("expected an invalid bug error, got %v", err)
	}
	if invalidIDBug != nil {
		t.Errorf("expected no bug, got: %v", invalidIDBug)
	}
}

func TestCreateBug(t *testing.T) {
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
		if r.Method != http.MethodPost {
			t.Errorf("incorrect method to create a bug: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/bug") {
			t.Errorf("incorrect path to create a bug: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		raw, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			http.Error(w, "500 Server Error", http.StatusInternalServerError)
			return
		}
		payload := &BugCreate{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("malformed JSONRPC payload: %s", string(raw))
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if _, err := w.Write([]byte(`{"id" : 12345}`)); err != nil {
			t.Fatalf("failed to send JSONRPC response: %v", err)
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	// this should create a new bug
	if id, err := client.CreateBug(&BugCreate{Description: "This is a test bug"}); err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	} else if id != 12345 {
		t.Errorf("expected id of 12345, got %d", id)
	}
}

func TestGetComments(t *testing.T) {
	commentsJSON := []byte(`{
		"bugs": {
		  "12345": {
			"comments": [
			  {
				"time": "2020-04-21T13:50:04Z",
				"text": "test bug to fix problem in removing from cc list.",
				"bug_id": 12345,
				"count": 0,
				"attachment_id": null,
				"is_private": false,
				"is_markdown" : true,
				"tags": [],
				"creator": "user@bugzilla.org",
				"creation_time": "2020-04-21T13:50:04Z",
				"id": 75
			  },
			  {
				"time": "2020-04-21T13:52:02Z",
				"text": "Bug appears to be fixed",
				"bug_id": 12345,
				"count": 1,
				"attachment_id": null,
				"is_private": false,
				"is_markdown" : true,
				"tags": [],
				"creator": "user2@bugzilla.org",
				"creation_time": "2020-04-21T13:52:02Z",
				"id": 76
			  }
			]
		  }
		},
		"comments": {}
	  }`)
	commentsStruct := []Comment{{
		ID:           75,
		BugID:        12345,
		AttachmentID: nil,
		Count:        0,
		Text:         "test bug to fix problem in removing from cc list.",
		Creator:      "user@bugzilla.org",
		Time:         time.Date(2020, time.April, 21, 13, 50, 04, 0, time.UTC),
		CreationTime: time.Date(2020, time.April, 21, 13, 50, 04, 0, time.UTC),
		IsPrivate:    false,
		IsMarkdown:   true,
		Tags:         []string{},
	}, {
		ID:           76,
		BugID:        12345,
		AttachmentID: nil,
		Count:        1,
		Text:         "Bug appears to be fixed",
		Creator:      "user2@bugzilla.org",
		Time:         time.Date(2020, time.April, 21, 13, 52, 02, 0, time.UTC),
		CreationTime: time.Date(2020, time.April, 21, 13, 52, 02, 0, time.UTC),
		IsPrivate:    false,
		IsMarkdown:   true,
		Tags:         []string{},
	}}
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
			t.Errorf("incorrect method to get bug comments: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/bug/") {
			t.Errorf("incorrect path to get bug comments: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/comment") {
			t.Errorf("incorrect path to get bug comments: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if id, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/bug/"), "/comment")); err != nil {
			t.Errorf("malformed bug id: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
		} else {
			if id == 12345 {
				w.Write(commentsJSON)
			} else if id == 2 {
				w.Write(bugAccessDenied)
			} else if id == 3 {
				w.Write(bugInvalidBugID)
			} else {
				http.Error(w, "404 Not Found", http.StatusNotFound)
			}
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	comments, err := client.GetComments(12345)
	if err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	}
	if diff := cmp.Diff(comments, commentsStruct); diff != "" {
		t.Errorf("got incorrect comments: %v", diff)
	}

	// this should 404
	otherBug, err := client.GetComments(1)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}
	if otherBug != nil {
		t.Errorf("expected no bug, got: %v", otherBug)
	}

	// this should return access denied
	accessDeniedBug, err := client.GetComments(2)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsAccessDenied(err) {
		t.Errorf("expected an access denied error, got %v", err)
	}
	if accessDeniedBug != nil {
		t.Errorf("expected no bug, got: %v", accessDeniedBug)
	}

	// this should return invalid Bug ID
	invalidIDBug, err := client.GetComments(3)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsInvalidBugID(err) {
		t.Errorf("expected an invalid bug error, got %v", err)
	}
	if invalidIDBug != nil {
		t.Errorf("expected no bug, got: %v", invalidIDBug)
	}
}

func TestCreateComment(t *testing.T) {
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
		if r.Method != http.MethodPost {
			t.Errorf("incorrect method to create a comment: %s", r.Method)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if !regexp.MustCompile(`^/rest/bug/\d+/comment$`).MatchString(r.URL.Path) {
			t.Errorf("incorrect path to create a comment: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		raw, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			http.Error(w, "500 Server Error", http.StatusInternalServerError)
			return
		}
		payload := &CommentCreate{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("malformed JSONRPC payload: %s", string(raw))
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		if _, err := w.Write([]byte(`{"id" : 12345}`)); err != nil {
			t.Fatalf("failed to send JSONRPC response: %v", err)
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	// this should create a new comment
	if id, err := client.CreateComment(&CommentCreate{ID: 2, Comment: "This is a test bug"}); err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	} else if id != 12345 {
		t.Errorf("expected id of 12345, got %d", id)
	}
}

func TestCloneBugStruct(t *testing.T) {
	testCases := []struct {
		name     string
		bug      Bug
		comments []Comment
		expected BugCreate
	}{{
		name: "Clone bug",
		bug: Bug{
			Alias:           []string{"this_is_an_alias"},
			AssignedTo:      "user@example.com",
			CC:              []string{"user2@example.com", "user3@example.com"},
			Component:       []string{"TestComponent"},
			Flags:           []Flag{{ID: 1, Name: "Test Flag"}},
			Groups:          []string{"group1"},
			ID:              123,
			Keywords:        []string{"segfault"},
			OperatingSystem: "Fedora",
			Platform:        "x86_64",
			Priority:        "unspecified",
			Product:         "testing product",
			QAContact:       "user3@example.com",
			Resolution:      "FIXED",
			Severity:        "Urgent",
			Status:          "VERIFIED",
			Summary:         "Segfault when opening program",
			TargetMilestone: "milestone1",
			Version:         []string{"31"},
		},
		comments: []Comment{{
			Text:      "There is a segfault that occurs when opening applications.",
			IsPrivate: true,
		}},
		expected: BugCreate{
			Alias:            []string{"this_is_an_alias"},
			AssignedTo:       "user@example.com",
			CC:               []string{"user2@example.com", "user3@example.com"},
			Component:        []string{"TestComponent"},
			Flags:            []Flag{{ID: 1, Name: "Test Flag"}},
			Groups:           []string{"group1"},
			Keywords:         []string{"segfault"},
			OperatingSystem:  "Fedora",
			Platform:         "x86_64",
			Priority:         "unspecified",
			Product:          "testing product",
			QAContact:        "user3@example.com",
			Severity:         "Urgent",
			Summary:          "Segfault when opening program",
			TargetMilestone:  "milestone1",
			Version:          []string{"31"},
			Description:      "+++ This bug was initially created as a clone of Bug #123 +++\n\nThere is a segfault that occurs when opening applications.",
			CommentIsPrivate: true,
		},
	}, {
		name: "Clone bug with multiple comments",
		bug: Bug{
			ID: 123,
		},
		comments: []Comment{{
			Text: "There is a segfault that occurs when opening applications.",
		}, {
			Text:         "This is another comment.",
			Time:         time.Date(2020, time.May, 7, 2, 3, 4, 0, time.UTC),
			CreationTime: time.Date(2020, time.May, 7, 2, 3, 4, 0, time.UTC),
			Tags:         []string{"description"},
			Creator:      "Test Commenter",
		}},
		expected: BugCreate{
			Description: `+++ This bug was initially created as a clone of Bug #123 +++

There is a segfault that occurs when opening applications.

--- Additional comment from Test Commenter on 2020-05-07 02:03:04 UTC ---

This is another comment.`,
		},
	}, {
		name: "Clone bug with one private comments",
		bug: Bug{
			ID: 123,
		},
		comments: []Comment{{
			Text: "There is a segfault that occurs when opening applications.",
		}, {
			Text:         "This is another comment.",
			Time:         time.Date(2020, time.May, 7, 2, 3, 4, 0, time.UTC),
			CreationTime: time.Date(2020, time.May, 7, 2, 3, 4, 0, time.UTC),
			IsPrivate:    true,
			Tags:         []string{"description"},
			Creator:      "Test Commenter",
		}},
		expected: BugCreate{
			Description: `+++ This bug was initially created as a clone of Bug #123 +++

There is a segfault that occurs when opening applications.

--- Additional comment from Test Commenter on 2020-05-07 02:03:04 UTC ---

This is another comment.`,
			CommentIsPrivate: true,
		},
	}}
	for _, testCase := range testCases {
		newBug := cloneBugStruct(&testCase.bug, nil, testCase.comments)
		if diff := cmp.Diff(*newBug, testCase.expected); diff != "" {
			t.Errorf("%s: Difference in expected BugCreate and actual: %s", testCase.name, diff)
		}
	}
	// test max length truncation
	bug := Bug{}
	baseComment := Comment{Text: "This is a test comment"}
	comments := []Comment{}
	// Make sure comments are at lest 65535 in total length
	for i := 0; i < (65535 / len(baseComment.Text)); i++ {
		comments = append(comments, baseComment)
	}
	newBug := cloneBugStruct(&bug, nil, comments)
	if len(newBug.Description) != 65535 {
		t.Errorf("Truncation error in cloneBug; expected description length of 65535, got %d", len(newBug.Description))
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
				if actual, expected := string(raw), `{"depends_on":{"add":[1705242]},"status":"UPDATED"}`; actual != expected {
					t.Errorf("got incorrect update: expected %v, got %v", expected, actual)
				}
			} else if id == 2 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, `{"documentation":"https://bugzilla.redhat.com/docs/en/html/api/index.html","error":true,"code":32000,"message":"Subcomponet is mandatory for the component 'Cloud Compute' in the product 'OpenShift Container Platform'."}`)
			} else {
				http.Error(w, "404 Not Found", http.StatusNotFound)
			}
		}
	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	update := BugUpdate{
		DependsOn: &IDUpdate{
			Add: []int{1705242},
		},
		Status: "UPDATED",
	}

	// this should run an update
	if err := client.UpdateBug(1705243, update); err != nil {
		t.Errorf("expected no error, but got one: %v", err)
	}

	// this should 404
	err := client.UpdateBug(1, update)
	if err == nil {
		t.Error("expected an error, but got none")
	} else if !IsNotFound(err) {
		t.Errorf("expected a not found error, got %v", err)
	}

	// this is a 200 with an error payload
	if err := client.UpdateBug(2, update); err == nil {
		t.Error("expected an error, but got none")
	}
}

func TestAddPullRequestAsExternalBug(t *testing.T) {
	var testCases = []struct {
		name            string
		trackerId       uint
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
			name:            "explicit tracker ID is used, update succeeds, makes a change",
			trackerId:       123,
			id:              17052430,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[17052430],"external_bugs":[{"ext_type_id":123,"ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"bugs":[{"alias":[],"changes":{"ext_bz_bug_map.ext_bz_bug_id":{"added":"Github org/repo/pull/1","removed":""}},"id":17052430}]}}`,
			expectedError:   false,
			expectedChanged: true,
		},
		{
			name:            "update succeeds, makes a change as part of multiple changes reported",
			id:              1705244,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705244],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"bugs":[{"alias":[],"changes":{"ext_bz_bug_map.ext_bz_bug_id":{"added":"Github org/repo/pull/1","removed":""}},"id":1705244},{"alias":[],"changes":{"ext_bz_bug_map.ext_bz_bug_id":{"added":"Github org/repo/pull/2","removed":""}},"id":1705244}]}}`,
			expectedError:   false,
			expectedChanged: true,
		},
		{
			name:            "update succeeds, makes no change",
			id:              1705245,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705245],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
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
		{
			name:            "update already made earlier, makes no change",
			id:              1705248,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.add_external_bug","params":[{"api_key":"api-key","bug_ids":[1705248],"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}],"id":"identifier"}`,
			response:        `{"error":{"code": 100500,"message":"DBD::Pg::db do failed: ERROR:  duplicate key value violates unique constraint \"ext_bz_bug_map_bug_id_idx\"\nDETAIL:  Key (bug_id, ext_bz_id, ext_bz_bug_id)=(1778894, 131, openshift/installer/pull/2728) already exists. [for Statement \"INSERT INTO ext_bz_bug_map (ext_description, ext_bz_id, ext_bz_bug_id, ext_priority, ext_last_updated, bug_id, ext_status) VALUES (?,?,?,?,?,?,?)\"]\n\u003cpre\u003e\n at /var/www/html/bugzilla/Bugzilla/Object.pm line 754.\n\tBugzilla::Object::insert_create_data('Bugzilla::Extension::ExternalBugs::Bug', 'HASH(0x55eec2747a30)') called at /loader/0x55eec2720cc0/Bugzilla/Extension/ExternalBugs/Bug.pm line 118\n\tBugzilla::Extension::ExternalBugs::Bug::create('Bugzilla::Extension::ExternalBugs::Bug', 'HASH(0x55eed47b6d20)') called at /var/www/html/bugzilla/extensions/ExternalBugs/Extension.pm line 858\n\tBugzilla::Extension::ExternalBugs::bug_start_of_update('Bugzilla::Extension::ExternalBugs=HASH(0x55eecf484038)', 'HASH(0x55eed09302e8)') called at /var/www/html/bugzilla/Bugzilla/Hook.pm line 21\n\tBugzilla::Hook::process('bug_start_of_update', 'HASH(0x55eed09302e8)') called at /var/www/html/bugzilla/Bugzilla/Bug.pm line 1168\n\tBugzilla::Bug::update('Bugzilla::Bug=HASH(0x55eed048b350)') called at /loader/0x55eec2720cc0/Bugzilla/Extension/ExternalBugs/WebService.pm line 80\n\tBugzilla::Extension::ExternalBugs::WebService::add_external_bug('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed38bd710)') called at (eval 5435) line 1\n\teval ' $procedure-\u003e{code}-\u003e($self, @params) \n;' called at /usr/share/perl5/vendor_perl/JSON/RPC/Legacy/Server.pm line 220\n\tJSON::RPC::Legacy::Server::_handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed1990ef0)') called at /var/www/html/bugzilla/Bugzilla/WebService/Server/JSONRPC.pm line 295\n\tBugzilla::WebService::Server::JSONRPC::_handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed1990ef0)') called at /usr/share/perl5/vendor_perl/JSON/RPC/Legacy/Server.pm line 126\n\tJSON::RPC::Legacy::Server::handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...') called at /var/www/html/bugzilla/Bugzilla/WebService/Server/JSONRPC.pm line 70\n\tBugzilla::WebService::Server::JSONRPC::handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...') called at /var/www/html/bugzilla/jsonrpc.cgi line 31\n\tModPerl::ROOT::Bugzilla::ModPerl::ResponseHandler::var_www_html_bugzilla_jsonrpc_2ecgi::handler('Apache2::RequestRec=SCALAR(0x55eed3231870)') called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 207\n\teval {...} called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 207\n\tModPerl::RegistryCooker::run('Bugzilla::ModPerl::ResponseHandler=HASH(0x55eed023da08)') called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 173\n\tModPerl::RegistryCooker::default_handler('Bugzilla::ModPerl::ResponseHandler=HASH(0x55eed023da08)') called at /usr/lib64/perl5/vendor_perl/ModPerl/Registry.pm line 32\n\tModPerl::Registry::handler('Bugzilla::ModPerl::ResponseHandler', 'Apache2::RequestRec=SCALAR(0x55eed3231870)') called at /var/www/html/bugzilla/mod_perl.pl line 139\n\tBugzilla::ModPerl::ResponseHandler::handler('Bugzilla::ModPerl::ResponseHandler', 'Apache2::RequestRec=SCALAR(0x55eed3231870)') called at (eval 5435) line 0\n\teval {...} called at (eval 5435) line 0\n\n\u003c/pre\u003e at /var/www/html/bugzilla/Bugzilla/Object.pm line 754.\n at /var/www/html/bugzilla/Bugzilla/Object.pm line 754.\n\tBugzilla::Object::insert_create_data('Bugzilla::Extension::ExternalBugs::Bug', 'HASH(0x55eec2747a30)') called at /loader/0x55eec2720cc0/Bugzilla/Extension/ExternalBugs/Bug.pm line 118\n\tBugzilla::Extension::ExternalBugs::Bug::create('Bugzilla::Extension::ExternalBugs::Bug', 'HASH(0x55eed47b6d20)') called at /var/www/html/bugzilla/extensions/ExternalBugs/Extension.pm line 858\n\tBugzilla::Extension::ExternalBugs::bug_start_of_update('Bugzilla::Extension::ExternalBugs=HASH(0x55eecf484038)', 'HASH(0x55eed09302e8)') called at /var/www/html/bugzilla/Bugzilla/Hook.pm line 21\n\tBugzilla::Hook::process('bug_start_of_update', 'HASH(0x55eed09302e8)') called at /var/www/html/bugzilla/Bugzilla/Bug.pm line 1168\n\tBugzilla::Bug::update('Bugzilla::Bug=HASH(0x55eed048b350)') called at /loader/0x55eec2720cc0/Bugzilla/Extension/ExternalBugs/WebService.pm line 80\n\tBugzilla::Extension::ExternalBugs::WebService::add_external_bug('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed38bd710)') called at (eval 5435) line 1\n\teval ' $procedure-\u003e{code}-\u003e($self, @params) \n;' called at /usr/share/perl5/vendor_perl/JSON/RPC/Legacy/Server.pm line 220\n\tJSON::RPC::Legacy::Server::_handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed1990ef0)') called at /var/www/html/bugzilla/Bugzilla/WebService/Server/JSONRPC.pm line 295\n\tBugzilla::WebService::Server::JSONRPC::_handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...', 'HASH(0x55eed1990ef0)') called at /usr/share/perl5/vendor_perl/JSON/RPC/Legacy/Server.pm line 126\n\tJSON::RPC::Legacy::Server::handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...') called at /var/www/html/bugzilla/Bugzilla/WebService/Server/JSONRPC.pm line 70\n\tBugzilla::WebService::Server::JSONRPC::handle('Bugzilla::WebService::Server::JSONRPC::Bugzilla::Extension::E...') called at /var/www/html/bugzilla/jsonrpc.cgi line 31\n\tModPerl::ROOT::Bugzilla::ModPerl::ResponseHandler::var_www_html_bugzilla_jsonrpc_2ecgi::handler('Apache2::RequestRec=SCALAR(0x55eed3231870)') called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 207\n\teval {...} called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 207\n\tModPerl::RegistryCooker::run('Bugzilla::ModPerl::ResponseHandler=HASH(0x55eed023da08)') called at /usr/lib64/perl5/vendor_perl/ModPerl/RegistryCooker.pm line 173\n\tModPerl::RegistryCooker::default_handler('Bugzilla::ModPerl::ResponseHandler=HASH(0x55eed023da08)') called at /usr/lib64/perl5/vendor_perl/ModPerl/Registry.pm line 32\n\tModPerl::Registry::handler('Bugzilla::ModPerl::ResponseHandler', 'Apache2::RequestRec=SCALAR(0x55eed3231870)') called at /var/www/html/bugzilla/mod_perl.pl line 139\n\tBugzilla::ModPerl::ResponseHandler::handler('Bugzilla::ModPerl::ResponseHandler', 'Apache2::RequestRec=SCALAR(0x55eed3231870)') called at (eval 5435) line 0\n\teval {...} called at (eval 5435) line 0"},"id":"identifier","result":null}`,
			expectedError:   false,
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
				if diff := cmp.Diff(string(raw), testCase.expectedPayload); diff != "" {
					t.Errorf("%s: got incorrect JSONRPC payload: %v", testCase.name, diff)
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
			client.githubExternalTrackerId = testCase.trackerId
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

func TestRemovePullRequestAsExternalBug(t *testing.T) {
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
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.remove_external_bug","params":[{"api_key":"api-key","bug_ids":[1705243],"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}]}}`,
			expectedError:   false,
			expectedChanged: true,
		},
		{
			name:            "update succeeds, makes a change as part of multiple changes reported",
			id:              1705244,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.remove_external_bug","params":[{"api_key":"api-key","bug_ids":[1705244],"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"external_bugs":[{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"},{"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/2"}]}}`,
			expectedError:   false,
			expectedChanged: true,
		},
		{
			name:            "update succeeds, makes no change",
			id:              1705245,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.remove_external_bug","params":[{"api_key":"api-key","bug_ids":[1705245],"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}],"id":"identifier"}`,
			response:        `{"error":null,"id":"identifier","result":{"external_bugs":[]}}`,
			expectedError:   false,
			expectedChanged: false,
		},
		{
			name:            "update fails, makes no change",
			id:              1705246,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.remove_external_bug","params":[{"api_key":"api-key","bug_ids":[1705246],"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}],"id":"identifier"}`,
			response:        `{"error":{"code": 100400,"message":"Invalid params for JSONRPC 1.0."},"id":"identifier","result":null}`,
			expectedError:   true,
			expectedChanged: false,
		},
		{
			name:            "get unrelated JSONRPC response",
			id:              1705247,
			expectedPayload: `{"jsonrpc":"1.0","method":"ExternalBugs.remove_external_bug","params":[{"api_key":"api-key","bug_ids":[1705247],"ext_type_url":"https://github.com/","ext_bz_bug_id":"org/repo/pull/1"}],"id":"identifier"}`,
			response:        `{"error":null,"id":"oops","result":{"external_bugs":[]}}`,
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
			Parameters []RemoveExternalBugParameters `json:"params"`
			ID         string                        `json:"id"`
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
			changed, err := client.RemovePullRequestAsExternalBug(testCase.id, "org", "repo", 1)
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

func TestIdentifierForPull(t *testing.T) {
	var testCases = []struct {
		name      string
		org, repo string
		num       int
		expected  string
	}{
		{
			name:     "normal works as expected",
			org:      "organization",
			repo:     "repository",
			num:      1234,
			expected: "organization/repository/pull/1234",
		},
	}

	for _, testCase := range testCases {
		if actual, expected := IdentifierForPull(testCase.org, testCase.repo, testCase.num), testCase.expected; actual != expected {
			t.Errorf("%s: got incorrect identifier, expected %s but got %s", testCase.name, expected, actual)
		}
	}
}

func TestPullFromIdentifier(t *testing.T) {
	var testCases = []struct {
		name                      string
		identifier                string
		expectedOrg, expectedRepo string
		expectedNum               int
		expectedErr               bool
		expectedNotPullErr        bool
	}{
		{
			name:         "normal works as expected",
			identifier:   "organization/repository/pull/1234",
			expectedOrg:  "organization",
			expectedRepo: "repository",
			expectedNum:  1234,
		},
		{
			name:        "wrong number of parts fails",
			identifier:  "organization/repository",
			expectedErr: true,
		},
		{
			name:               "not a pull fails but in an identifiable way",
			identifier:         "organization/repository/issue/1234",
			expectedErr:        true,
			expectedNotPullErr: true,
		},
		{
			name:        "not a number fails",
			identifier:  "organization/repository/pull/abcd",
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		org, repo, num, err := PullFromIdentifier(testCase.identifier)
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
		if testCase.expectedNotPullErr && !IsIdentifierNotForPullErr(err) {
			t.Errorf("%s: expected a notForPull error but got: %T", testCase.name, err)
		}
		if org != testCase.expectedOrg {
			t.Errorf("%s: got incorrect org, expected %s but got %s", testCase.name, testCase.expectedOrg, org)
		}
		if repo != testCase.expectedRepo {
			t.Errorf("%s: got incorrect repo, expected %s but got %s", testCase.name, testCase.expectedRepo, repo)
		}
		if num != testCase.expectedNum {
			t.Errorf("%s: got incorrect num, expected %d but got %d", testCase.name, testCase.expectedNum, num)
		}
	}
}

func TestGetExternalBugPRsOnBug(t *testing.T) {
	var testCases = []struct {
		name          string
		id            int
		response      string
		expectedError bool
		expectedPRs   []ExternalBug
	}{
		{
			name:          "no external bugs returns empty list",
			id:            1705243,
			response:      `{"bugs":[{"external_bugs":[]}],"faults":[]}`,
			expectedError: false,
		},
		{
			name:          "one external bug pointing to PR is found",
			id:            1705244,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 1705244,"ext_bz_bug_id":"org/repo/pull/1","type":{"url":"https://github.com/"}}]}],"faults":[]}`,
			expectedError: false,
			expectedPRs:   []ExternalBug{{Type: ExternalBugType{URL: "https://github.com/"}, BugzillaBugID: 1705244, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}},
		},
		{
			name:          "multiple external bugs pointing to PRs are found",
			id:            1705245,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 1705245,"ext_bz_bug_id":"org/repo/pull/1","type":{"url":"https://github.com/"}},{"bug_id": 1705245,"ext_bz_bug_id":"org/repo/pull/2","type":{"url":"https://github.com/"}}]}],"faults":[]}`,
			expectedError: false,
			expectedPRs:   []ExternalBug{{Type: ExternalBugType{URL: "https://github.com/"}, BugzillaBugID: 1705245, ExternalBugID: "org/repo/pull/1", Org: "org", Repo: "repo", Num: 1}, {Type: ExternalBugType{URL: "https://github.com/"}, BugzillaBugID: 1705245, ExternalBugID: "org/repo/pull/2", Org: "org", Repo: "repo", Num: 2}},
		},
		{
			name:          "external bugs pointing to issues are ignored",
			id:            1705246,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 1705246,"ext_bz_bug_id":"org/repo/issues/1","type":{"url":"https://github.com/"}}]}],"faults":[]}`,
			expectedError: false,
		},
		{
			name:          "external bugs pointing to other Bugzilla bugs are ignored",
			id:            1705247,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 3,"ext_bz_bug_id":"org/repo/pull/1","type":{"url":"https://github.com/"}}]}],"faults":[]}`,
			expectedError: false,
		},
		{
			name:          "external bugs pointing to other trackers are ignored",
			id:            1705248,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 1705248,"ext_bz_bug_id":"something","type":{"url":"https://bugs.tracker.com/"}}]}],"faults":[]}`,
			expectedError: false,
		},
		{
			name:          "external bugs pointing to invalid pulls cause an error",
			id:            1705249,
			response:      `{"bugs":[{"external_bugs":[{"bug_id": 1705249,"ext_bz_bug_id":"org/repo/pull/c","type":{"url":"https://github.com/"}}]}],"faults":[]}`,
			expectedError: true,
		},
	}
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
		if r.URL.Query().Get("include_fields") != "external_bugs" {
			t.Error("did not get external bugs passed in include_fields query parameter")
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
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
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/rest/bug/"))
		if err != nil {
			t.Errorf("malformed bug id: %s", r.URL.Path)
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		for _, testCase := range testCases {
			if id == testCase.id {
				if _, err := w.Write([]byte(testCase.response)); err != nil {
					t.Fatalf("%s: failed to send response: %v", testCase.name, err)
				}
				return
			}
		}

	}))
	defer testServer.Close()
	client := clientForUrl(testServer.URL)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prs, err := client.GetExternalBugPRsOnBug(testCase.id)
			if !testCase.expectedError && err != nil {
				t.Errorf("%s: expected no error, but got one: %v", testCase.name, err)
			}
			if testCase.expectedError && err == nil {
				t.Errorf("%s: expected an error, but got none", testCase.name)
			}
			if diff := cmp.Diff(prs, testCase.expectedPRs); diff != "" {
				t.Errorf("%s: got incorrect prs: %v", testCase.name, diff)
			}
		})
	}
}
func errorChecker(err error, t *testing.T) {
	if err != nil {
		t.Fatalf("Error while creating bugs for testing while calling the mocked endpoint!!")
	}
}
func TestGetAllClones(t *testing.T) {

	testcases := []struct {
		name            string
		bugs            []Bug
		bugToBeSearched Bug
		expectedClones  sets.Int
	}{
		{
			name: "Clones for the root node",
			bugs: []Bug{
				{Summary: "", ID: 1, Blocks: []int{2, 5}},
				{Summary: "", ID: 2, DependsOn: []int{1}, Blocks: []int{3}},
				{Summary: "", ID: 3, DependsOn: []int{2}},
				{Summary: "Not a clone", ID: 4, DependsOn: []int{1}},
				{Summary: "", ID: 5, DependsOn: []int{1}},
			},
			bugToBeSearched: Bug{Summary: "", ID: 1, Blocks: []int{2, 5}},
			expectedClones:  sets.NewInt(1, 2, 3, 5),
		},
		{
			name: "Clones for child of root",
			bugs: []Bug{
				{Summary: "", ID: 1, Blocks: []int{2, 5}},
				{Summary: "", ID: 2, DependsOn: []int{1}, Blocks: []int{3}},
				{Summary: "", ID: 3, DependsOn: []int{2}},
				{Summary: "Not a clone", ID: 4, DependsOn: []int{1}},
				{Summary: "", ID: 5, DependsOn: []int{1}},
			},
			bugToBeSearched: Bug{Summary: "", ID: 2, DependsOn: []int{1}, Blocks: []int{3}},
			expectedClones:  sets.NewInt(1, 2, 3, 5),
		},
		{
			name: "Clones for grandchild of root",
			bugs: []Bug{
				{Summary: "", ID: 1, Blocks: []int{2, 5}},
				{Summary: "", ID: 2, DependsOn: []int{1}, Blocks: []int{3}},
				{Summary: "", ID: 3, DependsOn: []int{2}},
				{Summary: "Not a clone", ID: 4, DependsOn: []int{1}},
				{Summary: "", ID: 5, DependsOn: []int{1}},
			},
			bugToBeSearched: Bug{Summary: "", ID: 3, DependsOn: []int{2}},
			expectedClones:  sets.NewInt(1, 2, 3, 5),
		},
		{
			name: "Clones when no clone is expected",
			bugs: []Bug{
				{Summary: "", ID: 1, Blocks: []int{2, 5}},
				{Summary: "", ID: 2, DependsOn: []int{1}, Blocks: []int{3}},
				{Summary: "", ID: 3, DependsOn: []int{2}},
				{Summary: "Not a clone", ID: 4, DependsOn: []int{1}},
				{Summary: "", ID: 5, DependsOn: []int{1}},
			},
			bugToBeSearched: Bug{Summary: "Not a clone", ID: 4, DependsOn: []int{1}},
			expectedClones:  sets.NewInt(4),
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &Fake{
				Bugs:        map[int]Bug{},
				BugComments: map[int][]Comment{},
			}
			for _, bug := range tc.bugs {
				fake.Bugs[bug.ID] = bug
			}
			bugCache := newBugDetailsCache()
			clones, err := getAllClones(fake, &tc.bugToBeSearched, bugCache)
			if err != nil {
				t.Errorf("Error occurred when none was expected: %v", err)
			}
			actualCloneSet := sets.NewInt()
			for _, clone := range clones {
				actualCloneSet.Insert(clone.ID)
			}
			if !tc.expectedClones.Equal(actualCloneSet) {
				t.Errorf("clones mismatch - expected %v, got %v", tc.expectedClones, actualCloneSet)
			}

		})

	}

}

func TestGetRootForClone(t *testing.T) {
	fake := &Fake{}
	fake.Bugs = map[int]Bug{}
	fake.BugComments = map[int][]Comment{}
	bug1Create := &BugCreate{
		Summary: "Dummy bug to test getAllClones",
	}
	bugDiffCreate := &BugCreate{
		Summary: "Different bug",
	}
	diffBugID, err := fake.CreateBug(bugDiffCreate)
	errorChecker(err, t)
	bug1ID, err := fake.CreateBug(bug1Create)
	if err != nil {
		t.Fatalf("Error while creating bug in Fake!\n")
	}
	idUpdate := &IDUpdate{
		Add: []int{diffBugID},
	}
	update := BugUpdate{
		DependsOn: idUpdate,
	}
	fake.UpdateBug(bug1ID, update)
	bug1, err := fake.GetBug(bug1ID)
	errorChecker(err, t)
	bug2ID, err := fake.CloneBug(bug1)
	errorChecker(err, t)
	bug2, err := fake.GetBug(bug2ID)
	errorChecker(err, t)
	bug3ID, err := fake.CloneBug(bug2)
	errorChecker(err, t)
	bug1, err = fake.GetBug(bug1ID)
	errorChecker(err, t)
	bug2, err = fake.GetBug(bug2ID)
	errorChecker(err, t)
	bug3, err := fake.GetBug(bug3ID)
	errorChecker(err, t)
	testcases := []struct {
		name         string
		bugPtr       *Bug
		expectedRoot int
	}{
		{
			"Root is itself",
			bug1,
			bug1ID,
		},
		{
			"Root is immediate parent",
			bug2,
			bug1ID,
		},
		{
			"Root is grandparent",
			bug3,
			bug1ID,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// this should run get the root
			root, err := getRootForClone(fake, tc.bugPtr)
			if err != nil {
				t.Errorf("Error occurred when error not expected: %v", err)
			}
			if root.ID != tc.expectedRoot {
				t.Errorf("ID of root incorrect.")
			}
		})
	}
}

func TestClone(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		original *Bug
		expected Bug
	}{
		{
			name:     "Simple",
			original: &Bug{ID: 1},
			expected: Bug{DependsOn: []int{1}},
		},
		{
			name:     "Copy blocks field",
			original: &Bug{ID: 1, Blocks: []int{0}},
			expected: Bug{DependsOn: []int{1}, Blocks: []int{0}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &Fake{Bugs: map[int]Bug{0: {}}, BugComments: map[int][]Comment{1: {{}}}}
			newID, err := clone(client, tc.original)
			if err != nil {
				t.Fatalf("cloning failed: %v", err)
			}
			tc.expected.ID = newID
			if diff := cmp.Diff(tc.expected, client.Bugs[newID]); diff != "" {
				t.Errorf("expected clone differs from actual clone: %s", diff)
			}
		})
	}
}
