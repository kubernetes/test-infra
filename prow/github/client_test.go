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

package github

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func getClient(url string) *Client {
	return &Client{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		base: url,
	}
}

func TestRequestRateLimit(t *testing.T) {
	var slept time.Duration
	timeSleep = func(d time.Duration) { slept = d }
	defer func() { timeSleep = time.Sleep }()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slept == 0 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(int(time.Now().Add(time.Second).Unix())))
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	resp, err := c.requestRetry(http.MethodGet, c.base, "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	} else if slept < time.Second {
		t.Errorf("Expected to sleep for at least a second, got %v", slept)
	}
}

func TestRetry404(t *testing.T) {
	var slept int
	timeSleep = func(d time.Duration) { slept++ }
	defer func() { timeSleep = time.Sleep }()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slept == 0 {
			http.Error(w, "404 Not Found", http.StatusNotFound)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	resp, err := c.requestRetry(http.MethodGet, c.base, "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
}

func TestBotName(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/user" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, "{\"login\": \"wowza\"}")
	}))
	c := getClient(ts.URL)
	botName, err := c.BotName()
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if botName != "wowza" {
		t.Errorf("Wrong bot name. Got %s, expected wowza.", botName)
	}
	ts.Close()
	botName, err = c.BotName()
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if botName != "wowza" {
		t.Errorf("Wrong bot name. Got %s, expected wowza.", botName)
	}
}

func TestIsMember(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/orgs/k8s/members/person" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	mem, err := c.IsMember("k8s", "person")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if !mem {
		t.Errorf("Should be member.")
	}
}

func TestCreateComment(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/comments" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ic IssueComment
		if err := json.Unmarshal(b, &ic); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if ic.Body != "hello" {
			t.Errorf("Wrong body: %s", ic.Body)
		}
		http.Error(w, "201 Created", http.StatusCreated)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.CreateComment("k8s", "kuber", 5, "hello"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestCreateCommentReaction(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/comments/5/reactions" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github.squirrel-girl-preview" {
			t.Errorf("Bad Accept header: %s", r.Header.Get("Accept"))
		}
		http.Error(w, "201 Created", http.StatusCreated)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.CreateCommentReaction("k8s", "kuber", 5, "+1"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestDeleteComment(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/comments/123" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.DeleteComment("k8s", "kuber", 123); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestGetPullRequest(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/12" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		pr := PullRequest{
			User: User{Login: "bla"},
		}
		b, err := json.Marshal(&pr)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	pr, err := c.GetPullRequest("k8s", "kuber", 12)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if pr.User.Login != "bla" {
		t.Errorf("Wrong user: %s", pr.User.Login)
	}
}

func TestGetPullRequestChanges(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/12/files" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		changes := []PullRequestChange{
			{Filename: "foo.txt"},
		}
		b, err := json.Marshal(&changes)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	cs, err := c.GetPullRequestChanges("k8s", "kuber", 12)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(cs) != 1 || cs[0].Filename != "foo.txt" {
		t.Errorf("Wrong result: %#v", cs)
	}
}

func TestGetRef(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/git/refs/heads/mastah" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"object": {"sha":"abcde"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	sha, err := c.GetRef("k8s", "kuber", "heads/mastah")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if sha != "abcde" {
		t.Errorf("Wrong sha: %s", sha)
	}
}

func TestCreateStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/statuses/abcdef" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var s Status
		if err := json.Unmarshal(b, &s); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if s.Context != "c" {
			t.Errorf("Wrong context: %s", s.Context)
		}
		http.Error(w, "201 Created", http.StatusCreated)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.CreateStatus("k8s", "kuber", "abcdef", Status{
		Context: "c",
	}); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestListIssueComments(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/repos/k8s/kuber/issues/15/comments" {
			ics := []IssueComment{{ID: 1}}
			b, err := json.Marshal(ics)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/someotherpath" {
			ics := []IssueComment{{ID: 2}}
			b, err := json.Marshal(ics)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	ics, err := c.ListIssueComments("k8s", "kuber", 15)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(ics) != 2 {
		t.Errorf("Expected two issues, found %d: %v", len(ics), ics)
	} else if ics[0].ID != 1 || ics[1].ID != 2 {
		t.Errorf("Wrong issue IDs: %v", ics)
	}
}

func TestAddLabel(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/labels" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ls []string
		if err := json.Unmarshal(b, &ls); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ls) != 1 {
			t.Errorf("Wrong length labels: %v", ls)
		} else if ls[0] != "yay" {
			t.Errorf("Wrong label: %s", ls[0])
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.AddLabel("k8s", "kuber", 5, "yay"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestRemoveLabel(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/labels/yay" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.RemoveLabel("k8s", "kuber", 5, "yay"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestAssignIssue(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/assignees" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string][]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if len(ps["assignees"]) == 3 {
			if ps["assignees"][0] != "george" || ps["assignees"][1] != "jungle" || ps["assignees"][2] != "not-in-the-org" {
				t.Errorf("Wrong assignees: %v", ps)
			}
		} else if len(ps["assignees"]) == 2 {
			if ps["assignees"][0] != "george" || ps["assignees"][1] != "jungle" {
				t.Errorf("Wrong assignees: %v", ps)
			}

		} else {
			t.Errorf("Wrong assignees length: %v", ps)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Issue{
			Assignees: []User{{Login: "george"}, {Login: "jungle"}, {Login: "ignore-other"}},
		})
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.AssignIssue("k8s", "kuber", 5, []string{"george", "jungle"}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := c.AssignIssue("k8s", "kuber", 5, []string{"george", "jungle", "not-in-the-org"}); err == nil {
		t.Errorf("Expected an error")
	} else if merr, ok := err.(MissingUsers); ok {
		if len(merr.Users) != 1 || merr.Users[0] != "not-in-the-org" {
			t.Errorf("Expected [not-in-the-org], not %v", merr.Users)
		}
	} else {
		t.Errorf("Expected MissingUsers error")
	}
}

func TestUnassignIssue(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/assignees" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string][]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if len(ps["assignees"]) == 3 {
			if ps["assignees"][0] != "george" || ps["assignees"][1] != "jungle" || ps["assignees"][2] != "perma-assignee" {
				t.Errorf("Wrong assignees: %v", ps)
			}
		} else if len(ps["assignees"]) == 2 {
			if ps["assignees"][0] != "george" || ps["assignees"][1] != "jungle" {
				t.Errorf("Wrong assignees: %v", ps)
			}

		} else {
			t.Errorf("Wrong assignees length: %v", ps)
		}
		json.NewEncoder(w).Encode(Issue{
			Assignees: []User{{Login: "perma-assignee"}, {Login: "ignore-other"}},
		})
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.UnassignIssue("k8s", "kuber", 5, []string{"george", "jungle"}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := c.UnassignIssue("k8s", "kuber", 5, []string{"george", "jungle", "perma-assignee"}); err == nil {
		t.Errorf("Expected an error")
	} else if merr, ok := err.(ExtraUsers); ok {
		if len(merr.Users) != 1 || merr.Users[0] != "perma-assignee" {
			t.Errorf("Expected [perma-assignee], not %v", merr.Users)
		}
	} else {
		t.Errorf("Expected ExtraUsers error")
	}
}

func TestReadPaginatedResults(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/label/foo" {
			objects := []Label{{Name: "foo"}}
			b, err := json.Marshal(objects)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/label/bar>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/label/bar" {
			objects := []Label{{Name: "bar"}}
			b, err := json.Marshal(objects)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	path := "/label/foo"
	var labels []Label
	err := c.readPaginatedResults(
		path,
		"",
		func() interface{} {
			return &[]Label{}
		},
		func(obj interface{}) {
			labels = append(labels, *(obj.(*[]Label))...)
		},
	)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(labels) != 2 {
		t.Errorf("Expected two labels, found %d: %v", len(labels), labels)
	} else if labels[0].Name != "foo" || labels[1].Name != "bar" {
		t.Errorf("Wrong label names: %v", labels)
	}
}

func TestListPullRequestComments(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/repos/k8s/kuber/pulls/15/comments" {
			prcs := []ReviewComment{{ID: 1}}
			b, err := json.Marshal(prcs)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/someotherpath" {
			prcs := []ReviewComment{{ID: 2}}
			b, err := json.Marshal(prcs)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	prcs, err := c.ListPullRequestComments("k8s", "kuber", 15)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(prcs) != 2 {
		t.Errorf("Expected two comments, found %d: %v", len(prcs), prcs)
	} else if prcs[0].ID != 1 || prcs[1].ID != 2 {
		t.Errorf("Wrong issue IDs: %v", prcs)
	}
}

func TestRequestReview(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/5/requested_reviewers" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string][]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Fatalf("Could not unmarshal request: %v", err)
		}
		if len(ps) != 1 {
			t.Fatalf("Wrong length patch: %v", ps)
		}
		switch len(ps["reviewers"]) {
		case 3:
			if ps["reviewers"][0] != "george" || ps["reviewers"][1] != "jungle" || ps["reviewers"][2] != "not-a-collaborator" {
				t.Errorf("Wrong reviewers: %v", ps)
			}
			//fall out of switch statement to bad reviewer case
		case 2:
			if ps["reviewers"][0] != "george" || ps["reviewers"][1] != "jungle" {
				t.Errorf("Wrong reviewers: %v", ps)
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(PullRequest{
				RequestedReviewers: []User{{Login: "george"}, {Login: "jungle"}, {Login: "ignore-other"}},
			})
			return
		case 1:
			if ps["reviewers"][0] != "not-a-collaborator" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(PullRequest{
					RequestedReviewers: []User{{Login: ps["reviewers"][0]}, {Login: "ignore-other"}},
				})
				return
			}
			//fall out of switch statement to bad reviewer case
		default:
			t.Errorf("Wrong reviewers length: %v", ps)
		}
		//bad reviewer case
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.RequestReview("k8s", "kuber", 5, []string{"george", "jungle"}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := c.RequestReview("k8s", "kuber", 5, []string{"george", "jungle", "not-a-collaborator"}); err == nil {
		t.Errorf("Expected an error")
	} else if merr, ok := err.(MissingUsers); ok {
		if len(merr.Users) != 1 || merr.Users[0] != "not-a-collaborator" {
			t.Errorf("Expected [not-a-collaborator], not %v", merr.Users)
		}
	} else {
		t.Errorf("Expected MissingUsers error")
	}
}

func TestUnrequestReview(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/5/requested_reviewers" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string][]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if len(ps["reviewers"]) == 3 {
			if ps["reviewers"][0] != "george" || ps["reviewers"][1] != "jungle" || ps["reviewers"][2] != "perma-reviewer" {
				t.Errorf("Wrong reviewers: %v", ps)
			}
		} else if len(ps["reviewers"]) == 2 {
			if ps["reviewers"][0] != "george" || ps["reviewers"][1] != "jungle" {
				t.Errorf("Wrong reviewers: %v", ps)
			}
		} else {
			t.Errorf("Wrong reviewers length: %v", ps)
		}
		json.NewEncoder(w).Encode(PullRequest{
			RequestedReviewers: []User{{Login: "perma-reviewer"}, {Login: "ignore-other"}},
		})
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.UnrequestReview("k8s", "kuber", 5, []string{"george", "jungle"}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := c.UnrequestReview("k8s", "kuber", 5, []string{"george", "jungle", "perma-reviewer"}); err == nil {
		t.Errorf("Expected an error")
	} else if merr, ok := err.(ExtraUsers); ok {
		if len(merr.Users) != 1 || merr.Users[0] != "perma-reviewer" {
			t.Errorf("Expected [perma-reviewer], not %v", merr.Users)
		}
	} else {
		t.Errorf("Expected ExtraUsers error")
	}
}

func TestCloseIssue(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if ps["state"] != "closed" {
			t.Errorf("Wrong state: %s", ps["state"])
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.CloseIssue("k8s", "kuber", 5); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestReopenIssue(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if ps["state"] != "open" {
			t.Errorf("Wrong state: %s", ps["state"])
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.ReopenIssue("k8s", "kuber", 5); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestClosePR(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/5" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if ps["state"] != "closed" {
			t.Errorf("Wrong state: %s", ps["state"])
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.ClosePR("k8s", "kuber", 5); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestReopenPR(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/5" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ps map[string]string
		if err := json.Unmarshal(b, &ps); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ps) != 1 {
			t.Errorf("Wrong length patch: %v", ps)
		} else if ps["state"] != "open" {
			t.Errorf("Wrong state: %s", ps["state"])
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.ReopenPR("k8s", "kuber", 5); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestFindIssues(t *testing.T) {
	cases := []struct {
		name  string
		sort  bool
		order bool
	}{
		{
			name: "simple query",
		},
		{
			name: "sort no order",
			sort: true,
		},
		{
			name:  "sort and order",
			sort:  true,
			order: true,
		},
	}

	issueNum := 5
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/search/issues" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		issueList := IssuesSearchResult{
			Total: 1,
			Issues: []Issue{
				{
					Number: issueNum,
					Title:  r.URL.RawQuery,
				},
			},
		}
		b, err := json.Marshal(&issueList)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)

	for _, tc := range cases {
		var result []Issue
		var err error
		sort := ""
		if tc.sort {
			sort = "sort-strategy"
		}
		if result, err = c.FindIssues("commit_hash", sort, tc.order); err != nil {
			t.Errorf("%s: didn't expect error: %v", tc.name, err)
		}
		if len(result) != 1 {
			t.Errorf("%s: unexpected number of results: %v", tc.name, len(result))
		}
		if result[0].Number != issueNum {
			t.Errorf("%s: expected issue number %+v, got %+v", tc.name, issueNum, result[0].Number)
		}
		if tc.sort && !strings.Contains(result[0].Title, "sort="+sort) {
			t.Errorf("%s: missing sort=%s from query: %s", tc.name, sort, result[0].Title)
		}
		if tc.order && !strings.Contains(result[0].Title, "order=asc") {
			t.Errorf("%s: missing order=asc from query: %s", tc.name, result[0].Title)
		}
	}
}

func TestGetFile(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/contents/foo.txt" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("Bad request query: %s", r.URL.RawQuery)
		}
		c := &Content{
			Content: base64.StdEncoding.EncodeToString([]byte("abcde")),
		}
		b, err := json.Marshal(&c)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if content, err := c.GetFile("k8s", "kuber", "foo.txt", ""); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if string(content) != "abcde" {
		t.Errorf("Wrong content -- expect: abcde, got: %s", string(content))
	}
}

func TestGetFileRef(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/contents/foo/bar.txt" {
			t.Errorf("Bad request path: %s", r.URL)
		}
		if r.URL.RawQuery != "ref=12345" {
			t.Errorf("Bad request query: %s", r.URL.RawQuery)
		}
		c := &Content{
			Content: base64.StdEncoding.EncodeToString([]byte("abcde")),
		}
		b, err := json.Marshal(&c)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if content, err := c.GetFile("k8s", "kuber", "foo/bar.txt", "12345"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if string(content) != "abcde" {
		t.Errorf("Wrong content -- expect: abcde, got: %s", string(content))
	}
}

// TestGetLabels tests both GetRepoLabels and GetIssueLabels.
func TestGetLabels(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		var labels []Label
		switch r.URL.Path {
		case "/repos/k8s/kuber/issues/5/labels":
			labels = []Label{{Name: "issue-label"}}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
		case "/repos/k8s/kuber/labels":
			labels = []Label{{Name: "repo-label"}}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
		case "/someotherpath":
			labels = []Label{{Name: "label2"}}
		default:
			t.Errorf("Bad request path: %s", r.URL.Path)
			return
		}
		b, err := json.Marshal(labels)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	labels, err := c.GetIssueLabels("k8s", "kuber", 5)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(labels) != 2 {
		t.Errorf("Expected two labels, found %d: %v", len(labels), labels)
	} else if labels[0].Name != "issue-label" || labels[1].Name != "label2" {
		t.Errorf("Wrong label names: %v", labels)
	}

	labels, err = c.GetRepoLabels("k8s", "kuber")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(labels) != 2 {
		t.Errorf("Expected two labels, found %d: %v", len(labels), labels)
	} else if labels[0].Name != "repo-label" || labels[1].Name != "label2" {
		t.Errorf("Wrong label names: %v", labels)
	}
}

func simpleTestServer(t *testing.T, path string, v interface{}) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else {
			t.Fatalf("Bad request path: %s", r.URL.Path)
		}
	}))
}

func TestListTeams(t *testing.T) {
	ts := simpleTestServer(t, "/orgs/foo/teams", []Team{{ID: 1}})
	defer ts.Close()
	c := getClient(ts.URL)
	teams, err := c.ListTeams("foo")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(teams) != 1 {
		t.Errorf("Expected one team, found %d: %v", len(teams), teams)
	} else if teams[0].ID != 1 {
		t.Errorf("Wrong team names: %v", teams)
	}
}

func TestListTeamMembers(t *testing.T) {
	ts := simpleTestServer(t, "/teams/1/members", []TeamMember{{Login: "foo"}})
	defer ts.Close()
	c := getClient(ts.URL)
	teamMembers, err := c.ListTeamMembers(1)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(teamMembers) != 1 {
		t.Errorf("Expected one team member, found %d: %v", len(teamMembers), teamMembers)
	} else if teamMembers[0].Login != "foo" {
		t.Errorf("Wrong team names: %v", teamMembers)
	}
}

func TestListCollaborators(t *testing.T) {
	ts := simpleTestServer(t, "/repos/org/repo/collaborators", []User{{Login: "foo"}, {Login: "bar"}})
	defer ts.Close()
	c := getClient(ts.URL)
	users, err := c.ListCollaborators("org", "repo")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(users) != 2 {
		t.Errorf("Expected two users, found %d: %v", len(users), users)
		return
	}
	if users[0].Login != "foo" {
		t.Errorf("Wrong user login for index 0: %v", users[0])
	}
	if users[1].Login != "bar" {
		t.Errorf("Wrong user login for index 1: %v", users[1])
	}
}
