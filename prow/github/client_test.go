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
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/diff"

	"k8s.io/test-infra/ghproxy/ghcache"
	"k8s.io/test-infra/prow/version"
)

type testTime struct {
	now   time.Time
	slept time.Duration
}

func (tt *testTime) Sleep(d time.Duration) {
	tt.slept = d
}
func (tt *testTime) Until(t time.Time) time.Duration {
	return t.Sub(tt.now)
}

func getClient(url string) *client {
	getToken := func() []byte {
		return []byte("")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return &client{
		logger: logrus.NewEntry(logger),
		delegate: &delegate{
			time:     &testTime{},
			throttle: throttler{throttlerDelegate: &throttlerDelegate{}},
			getToken: getToken,
			censor: func(content []byte) []byte {
				return content
			},
			client: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			},
			bases:         []string{url},
			maxRetries:    defaultMaxRetries,
			max404Retries: defaultMax404Retries,
			initialDelay:  defaultInitialDelay,
			maxSleepTime:  defaultMaxSleepTime,
		},
	}
}

func TestRequestRateLimit(t *testing.T) {
	tc := &testTime{now: time.Now()}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc.slept == 0 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(int(tc.now.Add(time.Second).Unix())))
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.time = tc
	resp, err := c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	} else if tc.slept < time.Second {
		t.Errorf("Expected to sleep for at least a second, got %v", tc.slept)
	}
}

func TestAbuseRateLimit(t *testing.T) {
	tc := &testTime{now: time.Now()}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc.slept == 0 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.time = tc
	resp, err := c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	} else if tc.slept < time.Second {
		t.Errorf("Expected to sleep for at least a second, got %v", tc.slept)
	}
}

func TestRetry404(t *testing.T) {
	tc := &testTime{now: time.Now()}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc.slept == 0 {
			http.Error(w, "404 Not Found", http.StatusNotFound)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.time = tc
	resp, err := c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
}

func TestRetryBase(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.initialDelay = time.Microsecond
	// One good endpoint:
	c.bases = []string{c.bases[0]}
	resp, err := c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	// Bad endpoint followed by good endpoint:
	c.bases = []string{"not-a-valid-base", c.bases[0]}
	resp, err = c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err != nil {
		t.Errorf("Error from request: %v", err)
	} else if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	// One bad endpoint:
	c.bases = []string{"not-a-valid-base"}
	_, err = c.requestRetry(http.MethodGet, "/", "", "", nil)
	if err == nil {
		t.Error("Expected an error from a request to an invalid base, but succeeded!?")
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

func TestCreateCommentCensored(t *testing.T) {
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
		} else if ic.Body != "CENSORED" {
			t.Errorf("Wrong body: %s", ic.Body)
		}
		http.Error(w, "201 Created", http.StatusCreated)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.delegate.censor = func(content []byte) []byte {
		return bytes.ReplaceAll(content, []byte("hello"), []byte("CENSORED"))
	}
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
	testCases := []struct {
		name              string
		githubResponse    []byte
		expectedSHA       string
		expectedError     string
		expectedErrorType error
	}{
		{
			name:           "single ref",
			githubResponse: []byte(`{"object": {"sha":"abcde"}}`),
			expectedSHA:    "abcde",
		},
		{
			name: "multiple refs, no match",
			githubResponse: []byte(`
[
  {
    "ref": "refs/heads/feature-a",
    "node_id": "MDM6UmVmcmVmcy9oZWFkcy9mZWF0dXJlLWE=",
    "url": "https://api.github.com/repos/octocat/Hello-World/git/refs/heads/feature-a",
    "object": {
      "type": "commit",
      "sha": "aa218f56b14c9653891f9e74264a383fa43fefbd",
      "url": "https://api.github.com/repos/octocat/Hello-World/git/commits/aa218f56b14c9653891f9e74264a383fa43fefbd"
    }
  },
  {
    "ref": "refs/heads/feature-b",
    "node_id": "MDM6UmVmcmVmcy9oZWFkcy9mZWF0dXJlLWI=",
    "url": "https://api.github.com/repos/octocat/Hello-World/git/refs/heads/feature-b",
    "object": {
      "type": "commit",
      "sha": "612077ae6dffb4d2fbd8ce0cccaa58893b07b5ac",
      "url": "https://api.github.com/repos/octocat/Hello-World/git/commits/612077ae6dffb4d2fbd8ce0cccaa58893b07b5ac"
    }
  }
]`),
			expectedError:     "query for org/repo ref \"heads/branch\" didn't match one but multiple refs: [refs/heads/feature-a refs/heads/feature-b]",
			expectedErrorType: GetRefTooManyResultsError{},
		},
		{
			name: "multiple refs with match",
			githubResponse: []byte(`
[
  {
    "ref": "refs/heads/branch",
    "node_id": "MDM6UmVmcmVmcy9oZWFkcy9mZWF0dXJlLWE=",
    "url": "https://api.github.com/repos/octocat/Hello-World/git/refs/heads/feature-a",
    "object": {
      "type": "commit",
      "sha": "aa218f56b14c9653891f9e74264a383fa43fefbd",
      "url": "https://api.github.com/repos/octocat/Hello-World/git/commits/aa218f56b14c9653891f9e74264a383fa43fefbd"
    }
  },
  {
    "ref": "refs/heads/feature-b",
    "node_id": "MDM6UmVmcmVmcy9oZWFkcy9mZWF0dXJlLWI=",
    "url": "https://api.github.com/repos/octocat/Hello-World/git/refs/heads/feature-b",
    "object": {
      "type": "commit",
      "sha": "612077ae6dffb4d2fbd8ce0cccaa58893b07b5ac",
      "url": "https://api.github.com/repos/octocat/Hello-World/git/commits/612077ae6dffb4d2fbd8ce0cccaa58893b07b5ac"
    }
  }
]`),
			expectedSHA: "aa218f56b14c9653891f9e74264a383fa43fefbd",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				if r.Method != http.MethodGet {
					t.Errorf("Bad method: %s", r.Method)
				}
				expectedPath := "/repos/org/repo/git/refs/heads/branch"
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got path %s", expectedPath, r.URL.Path)
				}
				w.Write(tc.githubResponse)
			}))
			defer ts.Close()

			c := getClient(ts.URL)
			var errMsg string
			sha, err := c.GetRef("org", "repo", "heads/branch")
			if err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedError {
				t.Fatalf("expected error %q, got error %q", tc.expectedError, err)
			}
			if !errors.Is(err, tc.expectedErrorType) {
				t.Errorf("expected error of type %T, got %T", tc.expectedErrorType, err)
			}
			if sha != tc.expectedSHA {
				t.Errorf("expected sha %q, got sha %q", tc.expectedSHA, sha)
			}
		})
	}
}

func TestDeleteRef(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/git/refs/heads/my-feature" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.DeleteRef("k8s", "kuber", "heads/my-feature"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestListFileCommits(t *testing.T) {
	githubResponse := []byte(`
[
  {
    "sha": "5833e02133690c6d608f66ef369e85865ede51de",
    "node_id": "MDY6Q29tbWl0Mjk2ODI0MjU5OjU4MzNlMDIxMzM2OTBjNmQ2MDhmNjZlZjM2OWU4NTg2NWVkZTUxZGU=",
    "commit": {
      "author": {
        "name": "Rustin Liu",
        "email": "rustin.liu@gmail.com",
        "date": "2021-01-17T15:29:04Z"
      },
      "committer": {
        "name": "GitHub",
        "email": "noreply@github.com",
        "date": "2021-01-17T15:29:04Z"
      },
      "message": "chore: update README.md (#281)\n\n* chore: update README.md\r\n\r\n* chore: update README.md",
      "tree": {
        "sha": "0cbce1df534461fb686a4d97f7e1549657f45594",
        "url": "https://api.github.com/repos/ti-community-infra/tichi/git/trees/0cbce1df534461fb686a4d97f7e1549657f45594"
      },
      "url": "https://api.github.com/repos/ti-community-infra/tichi/git/commits/5833e02133690c6d608f66ef369e85865ede51de",
      "comment_count": 0,
      "verification": {
        "verified": true,
        "reason": "valid",
        "signature": "-----BEGIN PGP SIGNATURE-----\n\nwsBcBAABCAAQBQJgBFfACRBK7hj4Ov3rIwAAdHIIAAdRO4WoBZPAcLREqPuSPX+h\nM1CpnIyytSoF8QesyCffLkCWbFwswMhPLM4aXW55EeSZKeEZyghb0Ehz0ZN1b3Zx\nJzFaHeydih2S5rTFk6MCn8ZY1oSZuA3spauqEJ8RxAoaHSmZ+Zq5ykQ9qar4rLto\n3LgpMkr+z137cTfeJ5iUQZPih8AsTS3/YAmUtPLMOanNKLtMDfD1xVj4luOqXz6X\nV0UFwQs/F+4HDvVAnwmh3soMxrKZ+ZOcSAGZYP6EjR75gaUy4EmRNUkVQxxNbJ11\nY4LV0j7ShFsRPQrSfBByhKL0Ug7uAiHGLGYCxW1wkULg4hArklS0YFFfuvZwhws=\n=ujFx\n-----END PGP SIGNATURE-----\n",
        "payload": "tree 0cbce1df534461fb686a4d97f7e1549657f45594\nparent 9e00ae5d353eb520b58a7440757f9d715572009f\nauthor Rustin Liu <rustin.liu@gmail.com> 1610897344 +0800\ncommitter GitHub <noreply@github.com> 1610897344 +0800\n\nchore: update README.md (#281)\n\n* chore: update README.md\r\n\r\n* chore: update README.md"
      }
    },
    "url": "https://api.github.com/repos/ti-community-infra/tichi/commits/5833e02133690c6d608f66ef369e85865ede51de",
    "html_url": "https://github.com/ti-community-infra/tichi/commit/5833e02133690c6d608f66ef369e85865ede51de",
    "comments_url": "https://api.github.com/repos/ti-community-infra/tichi/commits/5833e02133690c6d608f66ef369e85865ede51de/comments",
    "author": {
      "login": "hi-rustin",
      "id": 29879298,
      "node_id": "MDQ6VXNlcjI5ODc5Mjk4",
      "avatar_url": "https://avatars.githubusercontent.com/u/29879298?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/hi-rustin",
      "html_url": "https://github.com/hi-rustin",
      "followers_url": "https://api.github.com/users/hi-rustin/followers",
      "following_url": "https://api.github.com/users/hi-rustin/following{/other_user}",
      "gists_url": "https://api.github.com/users/hi-rustin/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/hi-rustin/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/hi-rustin/subscriptions",
      "organizations_url": "https://api.github.com/users/hi-rustin/orgs",
      "repos_url": "https://api.github.com/users/hi-rustin/repos",
      "events_url": "https://api.github.com/users/hi-rustin/events{/privacy}",
      "received_events_url": "https://api.github.com/users/hi-rustin/received_events",
      "type": "User",
      "site_admin": false
    },
    "committer": {
      "login": "web-flow",
      "id": 19864447,
      "node_id": "MDQ6VXNlcjE5ODY0NDQ3",
      "avatar_url": "https://avatars.githubusercontent.com/u/19864447?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/web-flow",
      "html_url": "https://github.com/web-flow",
      "followers_url": "https://api.github.com/users/web-flow/followers",
      "following_url": "https://api.github.com/users/web-flow/following{/other_user}",
      "gists_url": "https://api.github.com/users/web-flow/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/web-flow/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/web-flow/subscriptions",
      "organizations_url": "https://api.github.com/users/web-flow/orgs",
      "repos_url": "https://api.github.com/users/web-flow/repos",
      "events_url": "https://api.github.com/users/web-flow/events{/privacy}",
      "received_events_url": "https://api.github.com/users/web-flow/received_events",
      "type": "User",
      "site_admin": false
    },
    "parents": [
      {
        "sha": "9e00ae5d353eb520b58a7440757f9d715572009f",
        "url": "https://api.github.com/repos/ti-community-infra/tichi/commits/9e00ae5d353eb520b58a7440757f9d715572009f",
        "html_url": "https://github.com/ti-community-infra/tichi/commit/9e00ae5d353eb520b58a7440757f9d715572009f"
      }
    ]
  },
  {
    "sha": "68af84c32436c16564e1ac3c6ac36090d5d0baee",
    "node_id": "MDY6Q29tbWl0Mjk2ODI0MjU5OjY4YWY4NGMzMjQzNmMxNjU2NGUxYWMzYzZhYzM2MDkwZDVkMGJhZWU=",
    "commit": {
      "author": {
        "name": "Rustin Liu",
        "email": "rustin.liu@gmail.com",
        "date": "2021-01-14T08:34:14Z"
      },
      "committer": {
        "name": "GitHub",
        "email": "noreply@github.com",
        "date": "2021-01-14T08:34:14Z"
      },
      "message": "chore: rename project (#265)",
      "tree": {
        "sha": "853d8d79ab3fe498fcb415fb71ac8901de0272df",
        "url": "https://api.github.com/repos/ti-community-infra/tichi/git/trees/853d8d79ab3fe498fcb415fb71ac8901de0272df"
      },
      "url": "https://api.github.com/repos/ti-community-infra/tichi/git/commits/68af84c32436c16564e1ac3c6ac36090d5d0baee",
      "comment_count": 0,
      "verification": {
        "verified": true,
        "reason": "valid",
        "signature": "-----BEGIN PGP SIGNATURE-----\n\nwsBcBAABCAAQBQJgAAIGCRBK7hj4Ov3rIwAAdHIIAFdNBdgiG48GtiSXpbwXCpiq\nLTvCiJEkoRsuggNKlhvXvt3xEeVki8T0WcrKY70mkdNA11ie9PXdHLSowGyFYFRS\n9FwEUBKLBTYIyTpgvuBcUb17/M3QnobmIF1X66T/vxnqy8xvny6kRUk8qsxhLi6K\n5v61mHt3J5F+DwFhVaUVniMnUnQTdW+o9Utd8zEkKbT2pJkvi6cSAiQK6RqIBD7l\nZTBWgKtvrk75u1xBfqcTRRe00qmJdW+OmgPIhRKP9PGRLOrHUeLBs8Ov1YaSBa08\njd92057tt8tigiQBBgo6cTMlK0tupIf+YS5es3eNNVYkEdfxeZ8fRgwghfOLNAQ=\n=5STI\n-----END PGP SIGNATURE-----\n",
        "payload": "tree 853d8d79ab3fe498fcb415fb71ac8901de0272df\nparent a17a9df826165b832476c13c5f93ed8e7b58f2ce\nauthor Rustin Liu <rustin.liu@gmail.com> 1610613254 +0800\ncommitter GitHub <noreply@github.com> 1610613254 +0800\n\nchore: rename project (#265)\n\n"
      }
    },
    "url": "https://api.github.com/repos/ti-community-infra/tichi/commits/68af84c32436c16564e1ac3c6ac36090d5d0baee",
    "html_url": "https://github.com/ti-community-infra/tichi/commit/68af84c32436c16564e1ac3c6ac36090d5d0baee",
    "comments_url": "https://api.github.com/repos/ti-community-infra/tichi/commits/68af84c32436c16564e1ac3c6ac36090d5d0baee/comments",
    "author": {
      "login": "hi-rustin",
      "id": 29879298,
      "node_id": "MDQ6VXNlcjI5ODc5Mjk4",
      "avatar_url": "https://avatars.githubusercontent.com/u/29879298?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/hi-rustin",
      "html_url": "https://github.com/hi-rustin",
      "followers_url": "https://api.github.com/users/hi-rustin/followers",
      "following_url": "https://api.github.com/users/hi-rustin/following{/other_user}",
      "gists_url": "https://api.github.com/users/hi-rustin/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/hi-rustin/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/hi-rustin/subscriptions",
      "organizations_url": "https://api.github.com/users/hi-rustin/orgs",
      "repos_url": "https://api.github.com/users/hi-rustin/repos",
      "events_url": "https://api.github.com/users/hi-rustin/events{/privacy}",
      "received_events_url": "https://api.github.com/users/hi-rustin/received_events",
      "type": "User",
      "site_admin": false
    },
    "committer": {
      "login": "web-flow",
      "id": 19864447,
      "node_id": "MDQ6VXNlcjE5ODY0NDQ3",
      "avatar_url": "https://avatars.githubusercontent.com/u/19864447?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/web-flow",
      "html_url": "https://github.com/web-flow",
      "followers_url": "https://api.github.com/users/web-flow/followers",
      "following_url": "https://api.github.com/users/web-flow/following{/other_user}",
      "gists_url": "https://api.github.com/users/web-flow/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/web-flow/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/web-flow/subscriptions",
      "organizations_url": "https://api.github.com/users/web-flow/orgs",
      "repos_url": "https://api.github.com/users/web-flow/repos",
      "events_url": "https://api.github.com/users/web-flow/events{/privacy}",
      "received_events_url": "https://api.github.com/users/web-flow/received_events",
      "type": "User",
      "site_admin": false
    },
    "parents": [
      {
        "sha": "a17a9df826165b832476c13c5f93ed8e7b58f2ce",
        "url": "https://api.github.com/repos/ti-community-infra/tichi/commits/a17a9df826165b832476c13c5f93ed8e7b58f2ce",
        "html_url": "https://github.com/ti-community-infra/tichi/commit/a17a9df826165b832476c13c5f93ed8e7b58f2ce"
      }
    ]
  }
]`)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		expectedPath := "/repos/org/repo/commits"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got path %s", expectedPath, r.URL.Path)
		}
		expectRequestURI := "/repos/org/repo/commits?path=README.md&per_page=100"
		if r.URL.RequestURI() != expectRequestURI {
			t.Errorf("expected request URI %s, got request URI %s", expectRequestURI, r.URL.RequestURI())
		}
		w.Write(githubResponse)
	}))
	defer ts.Close()

	c := getClient(ts.URL)
	commits, err := c.ListFileCommits("org", "repo", "README.md")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(commits) != 2 {
		t.Errorf("Expected two commits, found %d: %v", len(commits), commits)
		return
	}
	if commits[0].Author.Login != "hi-rustin" {
		t.Errorf("Wrong author login for index 0: %v", commits[0])
	}
	if commits[1].Author.Login != "hi-rustin" {
		t.Errorf("Wrong author login for index 1: %v", commits[1])
	}
}

func TestGetSingleCommit(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/octocat/Hello-World/commits/6dcb09b5b57875f334f61aebed695e2e4193db5e" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{
			"commit": {
			  "tree": {
				"sha": "6dcb09b5b57875f334f61aebed695e2e4193db5e"
			  }
		        }
		  }`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	commit, err := c.GetSingleCommit("octocat", "Hello-World", "6dcb09b5b57875f334f61aebed695e2e4193db5e")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if commit.Commit.Tree.SHA != "6dcb09b5b57875f334f61aebed695e2e4193db5e" {
		t.Errorf("Wrong tree-hash: %s", commit.Commit.Tree.SHA)
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

func TestListIssues(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/repos/k8s/kuber/issues" {
			ics := []Issue{{Number: 1}}
			b, err := json.Marshal(ics)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/someotherpath" {
			ics := []Issue{{Number: 2}}
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
	ics, err := c.ListOpenIssues("k8s", "kuber")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(ics) != 2 {
		t.Errorf("Expected two issues, found %d: %v", len(ics), ics)
	} else if ics[0].Number != 1 || ics[1].Number != 2 {
		t.Errorf("Wrong issue IDs: %v", ics)
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

func addLabelHTTPServer(t *testing.T, org, repo string, number int, labels ...string) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != fmt.Sprintf("/repos/%s/%s/issues/%d/labels", org, repo, number) {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var ls []string
		if err := json.Unmarshal(b, &ls); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if len(ls) != len(labels) {
			t.Errorf("Wrong length labels: %v", ls)
		}

		for index, label := range labels {
			if ls[index] != label {
				t.Errorf("Wrong label: %s", ls[index])
			}
		}
	}))
}

func TestAddLabel(t *testing.T) {
	ts := addLabelHTTPServer(t, "k8s", "kuber", 5, "yay")
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.AddLabel("k8s", "kuber", 5, "yay"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestAddLabels(t *testing.T) {
	testCases := []struct {
		name   string
		org    string
		repo   string
		number int
		labels []string
	}{
		{
			name:   "one label",
			org:    "k8s",
			repo:   "kuber",
			number: 1,
			labels: []string{"one"},
		},
		{
			name:   "two label",
			org:    "k8s",
			repo:   "kuber",
			number: 2,
			labels: []string{"one", "two"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := addLabelHTTPServer(t, tc.org, tc.repo, tc.number, tc.labels...)
			defer ts.Close()

			c := getClient(ts.URL)
			if err := c.AddLabels(tc.org, tc.repo, tc.number, tc.labels...); err != nil {
				t.Errorf("Didn't expect error: %v", err)
			}
		})
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

func TestRemoveLabelFailsOnOtherThan404(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/issues/5/labels/yay" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "403 Forbidden", http.StatusForbidden)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	err := c.RemoveLabel("k8s", "kuber", 5, "yay")
	if err == nil {
		t.Errorf("Expected error but got none")
	}
}

func TestRemoveLabelNotFound(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message": "Label does not exist"}`, 404)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	err := c.RemoveLabel("any", "old", 3, "label")

	if err != nil {
		t.Fatalf("RemoveLabel expected no error, got one: %v", err)
	}
}

func TestNewNotFoundIsNotFound(t *testing.T) {
	if !IsNotFound(NewNotFound()) {
		t.Error("NewNotFound didn't return an error that was considered a NotFound")
	}
}

func TestIsNotFound(t *testing.T) {
	testCases := []struct {
		name       string
		code       int
		body       string
		isNotFound bool
	}{
		{
			name:       "should be not found when status code is 404",
			code:       404,
			body:       `{"message":"not found","errors":[{"resource":"fake resource","field":"fake field","code":"404","message":"status code 404"}]}`,
			isNotFound: true,
		},
		{
			name:       "should not be not found when status code is 200",
			code:       200,
			body:       `{"message": "ok"}`,
			isNotFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tc.body, tc.code)
			}))
			defer ts.Close()

			c := getClient(ts.URL)

			code, _, err := c.requestRaw(&request{
				method:    http.MethodGet,
				path:      fmt.Sprintf("/repos/%s/%s/branches/%s/protection", "org", "repo", "branch"),
				exitCodes: []int{200},
			})

			if code != tc.code {
				t.Fatalf("Expected code to be %d, but got %d", tc.code, code)
			}

			isNotFound := IsNotFound(err)

			if isNotFound != tc.isNotFound {
				t.Fatalf("Expected isNotFound to be %t, but got %t", tc.isNotFound, isNotFound)
			}
		})
	}
}

func TestIsNotFound_nested(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		err         error
		expectMatch bool
	}{
		{
			name:        "direct match",
			err:         requestError{ClientError: ClientError{Errors: []clientErrorSubError{{Message: "status code 404"}}}},
			expectMatch: true,
		},
		{
			name:        "direct, no match",
			err:         requestError{ClientError: ClientError{Errors: []clientErrorSubError{{Message: "status code 403"}}}},
			expectMatch: false,
		},
		{
			name:        "nested match",
			err:         fmt.Errorf("wrapping: %w", requestError{ClientError: ClientError{Errors: []clientErrorSubError{{Message: "status code 404"}}}}),
			expectMatch: true,
		},
		{
			name:        "nested, no match",
			err:         fmt.Errorf("wrapping: %w", requestError{ClientError: ClientError{Errors: []clientErrorSubError{{Message: "status code 403"}}}}),
			expectMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsNotFound(tc.err); result != tc.expectMatch {
				t.Errorf("expected match: %t, got match: %t", tc.expectMatch, result)
			}
		})
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
	type response struct {
		labels []Label
		next   string
	}
	cases := []struct {
		name           string
		baseSuffix     string
		initialPath    string
		responses      map[string]response
		expectedLabels []Label
	}{
		{
			name:        "regular pagination",
			initialPath: "/label/foo",
			responses: map[string]response{
				"/label/foo": {
					labels: []Label{{Name: "foo"}},
					next:   `<blorp>; rel="first", <https://%s/label/bar>; rel="next"`,
				},
				"/label/bar": {
					labels: []Label{{Name: "bar"}},
				},
			},
			expectedLabels: []Label{{Name: "foo"}, {Name: "bar"}},
		},
		{
			name:        "pagination with /api/v3 base suffix",
			initialPath: "/label/foo",
			baseSuffix:  "/api/v3",
			responses: map[string]response{
				"/api/v3/label/foo": {
					labels: []Label{{Name: "foo"}},
					next:   `<blorp>; rel="first", <https://%s/api/v3/label/bar>; rel="next"`,
				},
				"/api/v3/label/bar": {
					labels: []Label{{Name: "bar"}},
				},
			},
			expectedLabels: []Label{{Name: "foo"}, {Name: "bar"}},
		},
	}
	for _, tc := range cases {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Bad method: %s", r.Method)
			}
			if response, ok := tc.responses[r.URL.Path]; ok {
				b, err := json.Marshal(response.labels)
				if err != nil {
					t.Fatalf("Didn't expect error: %v", err)
				}
				if response.next != "" {
					w.Header().Set("Link", fmt.Sprintf(response.next, r.Host))
				}
				fmt.Fprint(w, string(b))
			} else {
				t.Errorf("Bad request path: %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		c := getClient(ts.URL)
		c.bases[0] = c.bases[0] + tc.baseSuffix
		var labels []Label
		err := c.readPaginatedResults(
			tc.initialPath,
			"",
			"",
			func() interface{} {
				return &[]Label{}
			},
			func(obj interface{}) {
				labels = append(labels, *(obj.(*[]Label))...)
			},
		)
		if err != nil {
			t.Errorf("%s: didn't expect error: %v", tc.name, err)
		} else {
			if !reflect.DeepEqual(labels, tc.expectedLabels) {
				t.Errorf("%s: expected %s, got %s", tc.name, tc.expectedLabels, labels)
			}
		}
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

func TestListReviews(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/repos/k8s/kuber/pulls/15/reviews" {
			reviews := []Review{{ID: 1}}
			b, err := json.Marshal(reviews)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/someotherpath" {
			reviews := []Review{{ID: 2}}
			b, err := json.Marshal(reviews)
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
	reviews, err := c.ListReviews("k8s", "kuber", 15)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(reviews) != 2 {
		t.Errorf("Expected two reviews, found %d: %v", len(reviews), reviews)
	} else if reviews[0].ID != 1 || reviews[1].ID != 2 {
		t.Errorf("Wrong review IDs: %v", reviews)
	}
}

func TestPrepareReviewersBody(t *testing.T) {
	var tests = []struct {
		name         string
		logins       []string
		expectedBody map[string][]string
	}{
		{
			name:         "one reviewer",
			logins:       []string{"george"},
			expectedBody: map[string][]string{"reviewers": {"george"}},
		},
		{
			name:         "three reviewers",
			logins:       []string{"george", "jungle", "chimp"},
			expectedBody: map[string][]string{"reviewers": {"george", "jungle", "chimp"}},
		},
		{
			name:         "one team",
			logins:       []string{"kubernetes/sig-testing-misc"},
			expectedBody: map[string][]string{"team_reviewers": {"sig-testing-misc"}},
		},
		{
			name:         "two teams",
			logins:       []string{"kubernetes/sig-testing-misc", "kubernetes/sig-testing-bugs"},
			expectedBody: map[string][]string{"team_reviewers": {"sig-testing-misc", "sig-testing-bugs"}},
		},
		{
			name:         "one team not in org",
			logins:       []string{"kubernetes/sig-testing-misc", "other-org/sig-testing-bugs"},
			expectedBody: map[string][]string{"team_reviewers": {"sig-testing-misc"}},
		},
		{
			name:         "mixed single",
			logins:       []string{"george", "kubernetes/sig-testing-misc"},
			expectedBody: map[string][]string{"reviewers": {"george"}, "team_reviewers": {"sig-testing-misc"}},
		},
		{
			name:         "mixed multiple",
			logins:       []string{"george", "kubernetes/sig-testing-misc", "kubernetes/sig-testing-bugs", "jungle", "chimp"},
			expectedBody: map[string][]string{"reviewers": {"george", "jungle", "chimp"}, "team_reviewers": {"sig-testing-misc", "sig-testing-bugs"}},
		},
	}
	for _, test := range tests {
		body, _ := prepareReviewersBody(test.logins, "kubernetes")
		if !reflect.DeepEqual(body, test.expectedBody) {
			t.Errorf("%s: got %s instead of %s", test.name, body, test.expectedBody)
		}
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
		if len(ps) < 1 || len(ps) > 2 {
			t.Fatalf("Wrong length patch: %v", ps)
		}
		if sets.NewString(ps["reviewers"]...).Has("not-a-collaborator") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		requestedReviewers := []User{}
		for _, reviewers := range ps {
			for _, reviewer := range reviewers {
				requestedReviewers = append(requestedReviewers, User{Login: reviewer})
			}
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PullRequest{
			RequestedReviewers: requestedReviewers,
		})
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.RequestReview("k8s", "kuber", 5, []string{"george", "jungle"}); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := c.RequestReview("k8s", "kuber", 5, []string{"george", "jungle", "k8s/team1"}); err != nil {
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
	if err := c.RequestReview("k8s", "kuber", 5, []string{"george", "jungle", "notk8s/team1"}); err == nil {
		t.Errorf("Expected an error")
	} else if merr, ok := err.(MissingUsers); ok {
		if len(merr.Users) != 1 || merr.Users[0] != "notk8s/team1" {
			t.Errorf("Expected [notk8s/team1], not %v", merr.Users)
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
			t.Fatalf("%s: unexpected number of results: %v", tc.name, len(result))
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

func TestCreateTeam(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/orgs/foo/teams" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var team Team
		switch err := json.Unmarshal(b, &team); {
		case err != nil:
			t.Errorf("Could not unmarshal request: %v", err)
		case team.Name == "":
			t.Errorf("client should reject empty names")
		case team.Name != "frobber":
			t.Errorf("Bad name: %s", team.Name)
		}
		team.Name = "hello"
		team.Description = "world"
		team.Privacy = "special"
		b, err = json.Marshal(team)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		w.WriteHeader(http.StatusCreated) // 201
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if _, err := c.CreateTeam("foo", Team{Name: ""}); err == nil {
		t.Errorf("client should reject empty name")
	}
	switch team, err := c.CreateTeam("foo", Team{Name: "frobber"}); {
	case err != nil:
		t.Errorf("unexpected error: %v", err)
	case team.Name != "hello":
		t.Errorf("bad name: %s", team.Name)
	case team.Description != "world":
		t.Errorf("bad description: %s", team.Description)
	case team.Privacy != "special":
		t.Errorf("bad privacy: %s", team.Privacy)
	}
}

func TestEditTeam(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/teams/63" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var team Team
		switch err := json.Unmarshal(b, &team); {
		case err != nil:
			t.Errorf("Could not unmarshal request: %v", err)
		case team.Name == "":
			t.Errorf("Bad name: %s", team.Name)
		}
		team.Name = "hello"
		team.Description = "world"
		team.Privacy = "special"
		b, err = json.Marshal(team)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		w.WriteHeader(http.StatusCreated) // 201
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if _, err := c.EditTeam("", Team{ID: 0, Name: "frobber"}); err == nil {
		t.Errorf("client should reject id 0")
	}
	switch team, err := c.EditTeam("", Team{ID: 63, Name: "frobber"}); {
	case err != nil:
		t.Errorf("unexpected error: %v", err)
	case team.Name != "hello":
		t.Errorf("bad name: %s", team.Name)
	case team.Description != "world":
		t.Errorf("bad description: %s", team.Description)
	case team.Privacy != "special":
		t.Errorf("bad privacy: %s", team.Privacy)
	}
}

func TestListTeamMembers(t *testing.T) {
	ts := simpleTestServer(t, "/teams/1/members", []TeamMember{{Login: "foo"}})
	defer ts.Close()
	c := getClient(ts.URL)
	teamMembers, err := c.ListTeamMembers("", 1, RoleAll)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(teamMembers) != 1 {
		t.Errorf("Expected one team member, found %d: %v", len(teamMembers), teamMembers)
	} else if teamMembers[0].Login != "foo" {
		t.Errorf("Wrong team names: %v", teamMembers)
	}
}

func TestIsCollaborator(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/collaborators/person" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	mem, err := c.IsCollaborator("k8s", "kuber", "person")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if !mem {
		t.Errorf("Should be member.")
	}
}

func TestListCollaborators(t *testing.T) {
	ts := simpleTestServer(t, "/repos/org/repo/collaborators", []User{
		{Login: "foo", Permissions: RepoPermissions{Pull: true}},
		{Login: "bar", Permissions: RepoPermissions{Push: true}},
	})
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
	if !reflect.DeepEqual(users[0].Permissions, RepoPermissions{Pull: true}) {
		t.Errorf("Wrong permissions for index 0: %v", users[0])
	}
	if users[1].Login != "bar" {
		t.Errorf("Wrong user login for index 1: %v", users[1])
	}
	if !reflect.DeepEqual(users[1].Permissions, RepoPermissions{Push: true}) {
		t.Errorf("Wrong permissions for index 1: %v", users[1])
	}
}

func TestListRepoTeams(t *testing.T) {
	expectedTeams := []Team{
		{ID: 1, Slug: "foo", Permission: RepoPull},
		{ID: 2, Slug: "bar", Permission: RepoPush},
		{ID: 3, Slug: "foobar", Permission: RepoAdmin},
	}
	ts := simpleTestServer(t, "/repos/org/repo/teams", expectedTeams)
	defer ts.Close()
	c := getClient(ts.URL)
	teams, err := c.ListRepoTeams("org", "repo")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(teams) != 3 {
		t.Errorf("Expected three teams, found %d: %v", len(teams), teams)
		return
	}
	if !reflect.DeepEqual(teams, expectedTeams) {
		t.Errorf("Wrong list of teams, expected: %v, got: %v", expectedTeams, teams)
	}
}
func TestListIssueEvents(t *testing.T) {
	ts := simpleTestServer(
		t,
		"/repos/org/repo/issues/1/events",
		[]ListedIssueEvent{
			{Event: IssueActionLabeled},
			{Event: IssueActionClosed},
		},
	)
	defer ts.Close()
	c := getClient(ts.URL)
	events, err := c.ListIssueEvents("org", "repo", 1)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(events) != 2 {
		t.Errorf("Expected two events, found %d: %v", len(events), events)
		return
	}
	if events[0].Event != IssueActionLabeled {
		t.Errorf("Wrong event for index 0: %v", events[0])
	}
	if events[1].Event != IssueActionClosed {
		t.Errorf("Wrong event for index 1: %v", events[1])
	}
}

func TestThrottle(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/org/repo/issues/1/events" {
			b, err := json.Marshal([]ListedIssueEvent{{Event: IssueActionClosed}})
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/repos/org/repo/issues/2/events" {
			w.Header().Set(ghcache.CacheModeHeader, string(ghcache.ModeRevalidated))
			b, err := json.Marshal([]ListedIssueEvent{{Event: IssueActionOpened}})
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			fmt.Fprint(w, string(b))
		} else {
			t.Fatalf("Bad request path: %s", r.URL.Path)
		}
	}))
	c := getClient(ts.URL)
	c.Throttle(1, 2)
	if c.client != &c.throttle {
		t.Errorf("Bad client %v, expecting %v", c.client, &c.throttle)
	}
	if len(c.throttle.throttle) != 2 {
		t.Fatalf("Expected two items in throttle channel, found %d", len(c.throttle.throttle))
	}
	if cap(c.throttle.throttle) != 2 {
		t.Fatalf("Expected throttle channel capacity of two, found %d", cap(c.throttle.throttle))
	}
	check := func(events []ListedIssueEvent, err error, expectedAction IssueEventAction) {
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(events) != 1 || events[0].Event != expectedAction {
			t.Errorf("Expected one %q event, found: %v", string(expectedAction), events)
		}
		if len(c.throttle.throttle) != 1 {
			t.Errorf("Expected one item in throttle channel, found %d", len(c.throttle.throttle))
		}
	}
	events, err := c.ListIssueEvents("org", "repo", 1)
	check(events, err, IssueActionClosed)
	// The following 2 calls should be refunded.
	events, err = c.ListIssueEvents("org", "repo", 2)
	check(events, err, IssueActionOpened)
	events, err = c.ListIssueEvents("org", "repo", 2)
	check(events, err, IssueActionOpened)

	// Check that calls are delayed while throttled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go func() {
		if _, err := c.ListIssueEvents("org", "repo", 1); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if _, err := c.ListIssueEvents("org", "repo", 1); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		cancel()
	}()
	slowed := false
	for ctx.Err() == nil {
		// Wait for the client to get throttled
		if atomic.LoadInt32(&c.throttle.slow) == 0 {
			continue
		}
		// Throttled, now add to the channel
		slowed = true
		select {
		case c.throttle.throttle <- time.Now(): // Add items to the channel
		case <-ctx.Done():
		}
	}
	if !slowed {
		t.Errorf("Never throttled")
	}
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("Expected context cancellation did not happen: %v", err)
	}
}

func TestGetBranches(t *testing.T) {
	ts := simpleTestServer(t, "/repos/org/repo/branches", []Branch{
		{Name: "master", Protected: false},
		{Name: "release-3.7", Protected: true},
	})
	defer ts.Close()
	c := getClient(ts.URL)
	branches, err := c.GetBranches("org", "repo", true)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	} else if len(branches) != 2 {
		t.Errorf("Expected two branches, found %d, %v", len(branches), branches)
		return
	}
	switch {
	case branches[0].Name != "master":
		t.Errorf("Wrong branch name for index 0: %v", branches[0])
	case branches[1].Name != "release-3.7":
		t.Errorf("Wrong branch name for index 1: %v", branches[1])
	case branches[1].Protected == false:
		t.Errorf("Wrong branch protection for index 1: %v", branches[1])
	}
}

func TestGetBranchProtection(t *testing.T) {
	contexts := []string{"foo-pr-test", "other"}
	pushers := []Team{{Slug: "movers"}, {Slug: "awesome-team"}, {Slug: "shakers"}}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/org/repo/branches/master/protection" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		bp := BranchProtection{
			RequiredStatusChecks: &RequiredStatusChecks{
				Contexts: contexts,
			},
			Restrictions: &Restrictions{
				Teams: pushers,
			},
		}
		b, err := json.Marshal(&bp)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	bp, err := c.GetBranchProtection("org", "repo", "master")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	switch {
	case bp.Restrictions == nil:
		t.Errorf("RestrictionsRequest unset")
	case bp.Restrictions.Teams == nil:
		t.Errorf("Teams unset")
	case len(bp.Restrictions.Teams) != len(pushers):
		t.Errorf("Bad teams: expected %v, got: %v", pushers, bp.Restrictions.Teams)
	case bp.RequiredStatusChecks == nil:
		t.Errorf("RequiredStatusChecks unset")
	case len(bp.RequiredStatusChecks.Contexts) != len(contexts):
		t.Errorf("Bad contexts: expected: %v, got: %v", contexts, bp.RequiredStatusChecks.Contexts)
	default:
		mc := map[string]bool{}
		for _, k := range bp.RequiredStatusChecks.Contexts {
			mc[k] = true
		}
		var missing []string
		for _, k := range contexts {
			if mc[k] != true {
				missing = append(missing, k)
			}
		}
		if n := len(missing); n > 0 {
			t.Errorf("missing %d required contexts: %v", n, missing)
		}
		mp := map[string]bool{}
		for _, k := range bp.Restrictions.Teams {
			mp[k.Slug] = true
		}
		missing = nil
		for _, k := range pushers {
			if mp[k.Slug] != true {
				missing = append(missing, k.Slug)
			}
		}
		if n := len(missing); n > 0 {
			t.Errorf("missing %d pushers: %v", n, missing)
		}
	}
}

// GetBranchProtection should return nil if the github API call
// returns 404 with "Branch not protected" message
func TestGetBranchProtection404BranchNotProtected(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/org/repo/branches/master/protection" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		ge := &githubError{
			Message: "Branch not protected",
		}
		b, err := json.Marshal(&ge)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		http.Error(w, string(b), http.StatusNotFound)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	bp, err := c.GetBranchProtection("org", "repo", "master")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if bp != nil {
		t.Errorf("Expected nil as BranchProtection object, got: %v", *bp)
	}
}

// GetBranchProtection should fail on any 404 which is NOT due to
// branch not being protected.
func TestGetBranchProtectionFailsOnOther404(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/org/repo/branches/master/protection" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		ge := &githubError{
			Message: "Not Found",
		}
		b, err := json.Marshal(&ge)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		http.Error(w, string(b), http.StatusNotFound)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	_, err := c.GetBranchProtection("org", "repo", "master")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestRemoveBranchProtection(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/org/repo/branches/master/protection" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		http.Error(w, "204 No Content", http.StatusNoContent)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.RemoveBranchProtection("org", "repo", "master"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestUpdateBranchProtection(t *testing.T) {
	cases := []struct {
		name string
		// TODO(fejta): expand beyond contexts/pushers
		contexts []string
		pushers  []string
		err      bool
	}{
		{
			name:     "both",
			contexts: []string{"foo-pr-test", "other"},
			pushers:  []string{"movers", "awesome-team", "shakers"},
			err:      false,
		},
	}

	for _, tc := range cases {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("Bad method: %s", r.Method)
			}
			if r.URL.Path != "/repos/org/repo/branches/master/protection" {
				t.Errorf("Bad request path: %s", r.URL.Path)
			}
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Could not read request body: %v", err)
			}
			var bpr BranchProtectionRequest
			if err := json.Unmarshal(b, &bpr); err != nil {
				t.Errorf("Could not unmarshal request: %v", err)
			}
			switch {
			case bpr.Restrictions != nil && bpr.Restrictions.Teams == nil:
				t.Errorf("Teams unset")
			case len(bpr.RequiredStatusChecks.Contexts) != len(tc.contexts):
				t.Errorf("Bad contexts: %v", bpr.RequiredStatusChecks.Contexts)
			case len(*bpr.Restrictions.Teams) != len(tc.pushers):
				t.Errorf("Bad teams: %v", *bpr.Restrictions.Teams)
			default:
				mc := map[string]bool{}
				for _, k := range tc.contexts {
					mc[k] = true
				}
				var missing []string
				for _, k := range bpr.RequiredStatusChecks.Contexts {
					if mc[k] != true {
						missing = append(missing, k)
					}
				}
				if n := len(missing); n > 0 {
					t.Errorf("%s: missing %d required contexts: %v", tc.name, n, missing)
				}
				mp := map[string]bool{}
				for _, k := range tc.pushers {
					mp[k] = true
				}
				missing = nil
				for _, k := range *bpr.Restrictions.Teams {
					if mp[k] != true {
						missing = append(missing, k)
					}
				}
				if n := len(missing); n > 0 {
					t.Errorf("%s: missing %d pushers: %v", tc.name, n, missing)
				}
			}
			http.Error(w, "200 OK", http.StatusOK)
		}))
		defer ts.Close()
		c := getClient(ts.URL)

		err := c.UpdateBranchProtection("org", "repo", "master", BranchProtectionRequest{
			RequiredStatusChecks: &RequiredStatusChecks{
				Contexts: tc.contexts,
			},
			Restrictions: &RestrictionsRequest{
				Teams: &tc.pushers,
			},
		})
		if tc.err && err == nil {
			t.Errorf("%s: expected error failed to occur", tc.name)
		}
		if !tc.err && err != nil {
			t.Errorf("%s: received unexpected error: %v", tc.name, err)
		}
	}
}

func TestClearMilestone(t *testing.T) {
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
		var issue Issue
		if err := json.Unmarshal(b, &issue); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if issue.Milestone.Title != "" {
			t.Errorf("Milestone title not empty: %v", issue.Milestone.Title)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.ClearMilestone("k8s", "kuber", 5); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestSetMilestone(t *testing.T) {
	newMilestone := 42
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
		var issue struct {
			Milestone *int `json:"milestone,omitempty"`
		}
		if err := json.Unmarshal(b, &issue); err != nil {
			t.Fatalf("Could not unmarshal request: %v", err)
		}
		if issue.Milestone == nil {
			t.Fatal("Milestone was not set.")
		}
		if *issue.Milestone != newMilestone {
			t.Errorf("Expected milestone to be set to %d, but got %d.", newMilestone, *issue.Milestone)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.SetMilestone("k8s", "kuber", 5, newMilestone); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestListMilestones(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/milestones" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err, _ := c.ListMilestones("k8s", "kuber"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestListPRCommits(t *testing.T) {
	ts := simpleTestServer(t, "/repos/theorg/therepo/pulls/3/commits",
		[]RepositoryCommit{
			{SHA: "sha"},
			{SHA: "sha2"},
		})
	defer ts.Close()
	c := getClient(ts.URL)
	if commits, err := c.ListPRCommits("theorg", "therepo", 3); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else {
		if len(commits) != 2 {
			t.Errorf("Expected 2 commits to be returned, but got %d", len(commits))
		}
	}
}

func TestCombinedStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path == "/repos/k8s/kuber/commits/SHA/status" {
			statuses := CombinedStatus{
				SHA:      "SHA",
				Statuses: []Status{{Context: "foo"}},
			}
			b, err := json.Marshal(statuses)
			if err != nil {
				t.Fatalf("Didn't expect error: %v", err)
			}
			w.Header().Set("Link", fmt.Sprintf(`<blorp>; rel="first", <https://%s/someotherpath>; rel="next"`, r.Host))
			fmt.Fprint(w, string(b))
		} else if r.URL.Path == "/someotherpath" {
			statuses := CombinedStatus{
				SHA:      "SHA",
				Statuses: []Status{{Context: "bar"}},
			}
			b, err := json.Marshal(statuses)
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
	combined, err := c.GetCombinedStatus("k8s", "kuber", "SHA")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if combined.SHA != "SHA" {
		t.Errorf("Expected SHA 'SHA', found %s", combined.SHA)
	} else if len(combined.Statuses) != 2 {
		t.Errorf("Expected two statuses, found %d: %v", len(combined.Statuses), combined.Statuses)
	} else if combined.Statuses[0].Context != "foo" || combined.Statuses[1].Context != "bar" {
		t.Errorf("Wrong review IDs: %v", combined.Statuses)
	}
}

func TestCreateRepo(t *testing.T) {
	org := "org"
	usersRepoName := "users-repository"
	orgsRepoName := "orgs-repository"
	repoDesc := "description of users-repository"
	testCases := []struct {
		description string
		isUser      bool
		repo        RepoCreateRequest
		statusCode  int

		expectError bool
		expectRepo  *FullRepo
	}{
		{
			description: "create repo as user",
			isUser:      true,
			repo: RepoCreateRequest{
				RepoRequest: RepoRequest{
					Name:        &usersRepoName,
					Description: &repoDesc,
				},
			},
			statusCode: http.StatusCreated,
			expectRepo: &FullRepo{
				Repo: Repo{
					Name:        "users-repository",
					Description: "CREATED",
				},
			},
		},
		{
			description: "create repo as org",
			isUser:      false,
			repo: RepoCreateRequest{
				RepoRequest: RepoRequest{
					Name:        &orgsRepoName,
					Description: &repoDesc,
				},
			},
			statusCode: http.StatusCreated,
			expectRepo: &FullRepo{
				Repo: Repo{
					Name:        "orgs-repository",
					Description: "CREATED",
				},
			},
		},
		{
			description: "errors are handled",
			isUser:      false,
			repo: RepoCreateRequest{
				RepoRequest: RepoRequest{
					Name:        &orgsRepoName,
					Description: &repoDesc,
				},
			},
			statusCode:  http.StatusForbidden,
			expectError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("Bad method: %s", r.Method)
				}
				if tc.isUser && r.URL.Path != "/user/repos" {
					t.Errorf("Bad request path to create user-owned repo: %s", r.URL.Path)
				} else if !tc.isUser && r.URL.Path != "/orgs/org/repos" {
					t.Errorf("Bad request path to create org-owned repo: %s", r.URL.Path)
				}
				b, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Could not read request body: %v", err)
				}
				var repo Repo
				switch err := json.Unmarshal(b, &repo); {
				case err != nil:
					t.Errorf("Could not unmarshal request: %v", err)
				case repo.Name == "":
					t.Errorf("client should reject empty names")
				}
				repo.Description = "CREATED"
				b, err = json.Marshal(repo)
				if err != nil {
					t.Fatalf("Didn't expect error: %v", err)
				}
				w.WriteHeader(tc.statusCode) // 201
				fmt.Fprint(w, string(b))
			}))
			defer ts.Close()
			c := getClient(ts.URL)
			switch repo, err := c.CreateRepo(org, tc.isUser, tc.repo); {
			case err != nil && !tc.expectError:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.expectError:
				t.Errorf("expected error, but got none")
			case err == nil && !reflect.DeepEqual(repo, tc.expectRepo):
				t.Errorf("%s: repo differs from expected:\n%s", tc.description, diff.ObjectReflectDiff(tc.expectRepo, repo))
			}
		})
	}
}

func TestUpdateRepo(t *testing.T) {
	org := "org"
	repoName := "repository"
	yes := true
	testCases := []struct {
		description string
		repo        RepoUpdateRequest
		statusCode  int

		expectError bool
		expectRepo  *FullRepo
	}{
		{
			description: "Update repository",
			repo: RepoUpdateRequest{
				RepoRequest: RepoRequest{
					Name: &repoName,
				},
				Archived: &yes,
			},
			statusCode: http.StatusOK,
			expectRepo: &FullRepo{
				Repo: Repo{
					Name:        "repository",
					Description: "UPDATED",
					Archived:    true,
				},
			},
		},
		{
			description: "errors are handled",
			repo: RepoUpdateRequest{
				RepoRequest: RepoRequest{
					Name: &repoName,
				},
				Archived: &yes,
			},
			statusCode:  http.StatusForbidden,
			expectError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Errorf("Bad method: %s (expected %s)", r.Method, http.MethodPatch)
				}
				expectedPath := "/repos/org/repository"
				if r.URL.Path != expectedPath {
					t.Errorf("Bad request path to create user-owned repo: %s (expected %s)", r.URL.Path, expectedPath)
				}
				b, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Could not read request body: %v", err)
				}
				var repo Repo
				switch err := json.Unmarshal(b, &repo); {
				case err != nil:
					t.Errorf("Could not unmarshal request: %v", err)
				case repo.Name == "":
					t.Errorf("client should reject empty names")
				}
				repo.Description = "UPDATED"
				b, err = json.Marshal(repo)
				if err != nil {
					t.Fatalf("Didn't expect error: %v", err)
				}
				w.WriteHeader(tc.statusCode) // 200
				fmt.Fprint(w, string(b))
			}))
			defer ts.Close()
			c := getClient(ts.URL)
			switch repo, err := c.UpdateRepo(org, repoName, tc.repo); {
			case err != nil && !tc.expectError:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.expectError:
				t.Errorf("expected error, but got none")
			case err == nil && !reflect.DeepEqual(repo, tc.expectRepo):
				t.Errorf("%s: repo differs from expected:\n%s", tc.description, diff.ObjectReflectDiff(tc.expectRepo, repo))
			}
		})
	}
}

type fakeHttpClient struct {
	received []*http.Request
}

func (fhc *fakeHttpClient) Do(req *http.Request) (*http.Response, error) {
	if fhc.received == nil {
		fhc.received = []*http.Request{}
	}
	fhc.received = append(fhc.received, req)
	return &http.Response{}, nil
}

func TestAuthHeaderGetsSet(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		mod            func(*client)
		expectedHeader http.Header
	}{
		{
			name: "Empty token, no auth header",
			mod:  func(c *client) { c.getToken = func() []byte { return []byte{} } },
		},
		{
			name:           "Token, auth header",
			mod:            func(c *client) { c.getToken = func() []byte { return []byte("sup") } },
			expectedHeader: http.Header{"Authorization": []string{"Bearer sup"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeHttpClient{}
			c := &client{delegate: &delegate{client: fake}, logger: logrus.NewEntry(logrus.New())}
			tc.mod(c)
			if _, err := c.doRequest("POST", "/hello", "", "", nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectedHeader == nil {
				tc.expectedHeader = http.Header{}
			}
			tc.expectedHeader["Accept"] = []string{"application/vnd.github.v3+json"}

			// Bazel injects some stuff in here, exclude it from comparison so both bazel test
			// and go test yield the same result.
			delete(fake.received[0].Header, "User-Agent")
			if diff := cmp.Diff(tc.expectedHeader, fake.received[0].Header); diff != "" {
				t.Errorf("expected header differs from actual: %s", diff)
			}
		})
	}
}

func TestListTeamRepos(t *testing.T) {
	ts := simpleTestServer(t, "/teams/1/repos",
		[]Repo{
			{
				Name:        "repo-bar",
				Permissions: RepoPermissions{Pull: true},
			},
			{
				Name: "repo-invalid-permission-level",
			},
		},
	)
	defer ts.Close()
	c := getClient(ts.URL)
	repos, err := c.ListTeamRepos("", 1)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(repos) != 1 {
		t.Errorf("Expected one repo, found %d: %v", len(repos), repos)
	} else if repos[0].Name != "repo-bar" {
		t.Errorf("Wrong repos: %v", repos)
	}
}

func TestCreateFork(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/forks" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		w.WriteHeader(202)
		w.Write([]byte(`{"name":"other"}`))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if name, err := c.CreateFork("k8s", "kuber"); err != nil {
		t.Errorf("Unexpected error: %v", err)
	} else {
		if name != "other" {
			t.Errorf("Unexpected fork name: %v", name)
		}
	}
}

func TestToCurl(t *testing.T) {
	testCases := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name:     "Authorization Header with bearer type gets masked",
			request:  &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "api.github.com"}, Header: http.Header{"Authorization": []string{"Bearer secret-token"}}},
			expected: `curl -k -v -XGET  -H "Authorization: Bearer <masked>" 'https://api.github.com'`,
		},
		{
			name:     "Authorization Header with unknown type gets masked",
			request:  &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "api.github.com"}, Header: http.Header{"Authorization": []string{"Definitely-not-valid secret-token"}}},
			expected: `curl -k -v -XGET  -H "Authorization: <masked>" 'https://api.github.com'`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if result := toCurl(tc.request); result != tc.expected {
				t.Errorf("result %s differs from expected %s", result, tc.expected)
			}
		})
	}
}

type testRoundTripper struct {
	rt func(*http.Request) (*http.Response, error)
}

func (rt testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt.rt(r)
}

// TestAllMethodsThatDoRequestSetOrgHeader uses reflect to find all methods of the Client and
// their arguments and calls them with an empty argument, then verifies via a RoundTripper that
// all requests made had an org header set.
func TestAllMethodsThatDoRequestSetOrgHeader(t *testing.T) {
	_, ghClient := NewAppsAuthClientWithFields(logrus.Fields{}, func(_ []byte) []byte { return nil }, "some-app-id", func() *rsa.PrivateKey { return nil }, "", "")
	clientType := reflect.TypeOf(ghClient)
	stringType := reflect.TypeOf("")
	stringValue := reflect.ValueOf("org")

	toSkip := sets.NewString(
		// Doesn't support github apps
		"Query",
		// They fetch the user, which doesn't exist in case of github app.
		// TODO: Split the search query by org when app auth is used
		"FindIssues",
	)

	for i := 0; i < clientType.NumMethod(); i++ {
		if toSkip.Has(clientType.Method(i).Name) {
			continue
		}
		t.Run(clientType.Method(i).Name, func(t *testing.T) {

			checkingRoundTripper := testRoundTripper{func(r *http.Request) (*http.Response, error) {
				if !strings.HasPrefix(r.URL.Path, "/app") {
					var orgVal string
					if v := r.Context().Value(githubOrgHeaderKey); v != nil {
						orgVal = v.(string)
					}
					if orgVal != "org" {
						t.Errorf("Request didn't have %s header set to 'org'", githubOrgHeaderKey)
					}
				}
				return &http.Response{Body: ioutil.NopCloser(&bytes.Buffer{})}, nil
			}}

			ghClient.(*client).client.(*http.Client).Transport = checkingRoundTripper
			ghClient.(*client).gqlc.(*graphQLGitHubAppsAuthClientWrapper).Client = githubv4.NewClient(&http.Client{Transport: checkingRoundTripper})
			clientValue := reflect.ValueOf(ghClient)

			var args []reflect.Value
			// First arg is self, so start with second arg
			for j := 1; j < clientType.Method(i).Func.Type().NumIn(); j++ {
				arg := reflect.New(clientType.Method(i).Func.Type().In(j)).Elem()
				if arg.Kind() == reflect.Ptr && arg.IsNil() {
					arg.Set(reflect.New(arg.Type().Elem()))
				}

				if arg.Type() == stringType {
					arg.Set(stringValue)
				}

				// We can not deal with interface types genererically, as there
				// is no automatic way to figure out the concrete values they
				// can or should be set to.
				if arg.Type().String() == "context.Context" {
					arg.Set(reflect.ValueOf(context.Background()))
				}
				if arg.Type().String() == "interface {}" {
					arg.Set(reflect.ValueOf(map[string]interface{}{}))
				}

				// Just set all strings to a nonEmpty string, otherwise the header will not get set
				args = append(args, arg)
			}

			if clientType.Method(i).Type.IsVariadic() {
				args[len(args)-1] = reflect.New(args[len(args)-1].Type().Elem()).Elem()
			}

			// We don't care about the result at all, the verification happens via the roundTripper
			_ = clientValue.Method(i).Call(args)
		})
	}
}

func TestBotUserChecker(t *testing.T) {
	const savedLogin = "botName"
	testCases := []struct {
		name         string
		checkFor     string
		usesAppsAuth bool
		expectMatch  bool
	}{
		{
			name:         "Bot suffix with apps auth is recognized",
			checkFor:     savedLogin + "[bot]",
			usesAppsAuth: true,
			expectMatch:  true,
		},
		{
			name:         "No suffix with apps auth is recognized",
			checkFor:     savedLogin,
			usesAppsAuth: true,
			expectMatch:  true,
		},
		{
			name:         "No suffix without apps auth is recognized",
			checkFor:     savedLogin,
			usesAppsAuth: false,
			expectMatch:  true,
		},
		{
			name:         "Suffix without apps auth is not recognized",
			checkFor:     savedLogin + "[bot]",
			usesAppsAuth: false,
			expectMatch:  false,
		},
	}

	for _, tc := range testCases {
		c := &client{delegate: &delegate{usesAppsAuth: tc.usesAppsAuth, userData: &UserData{Login: savedLogin}}}

		checker, err := c.BotUserChecker()
		if err != nil {
			t.Fatalf("failed to get user checker: %v", err)
		}
		if actualMatch := checker(tc.checkFor); actualMatch != tc.expectMatch {
			t.Errorf("expect match: %t, got match: %t", tc.expectMatch, actualMatch)
		}
	}
}

func TestV4ClientSetsUserAgent(t *testing.T) {
	// Make sure this is deterministic in tests
	version.Version = "0"
	var expectedUserAgent string
	roundTripper := testRoundTripper{func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("User-Agent"); got != expectedUserAgent {
			return nil, fmt.Errorf("expected User-Agent %q, got %q", expectedUserAgent, got)
		}
		return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(bytes.NewBufferString("{}"))}, nil
	}}

	_, client := newClient(
		logrus.Fields{},
		func() []byte { return nil },
		func(b []byte) []byte { return b },
		"",
		nil,
		"",
		false,
		nil,
		roundTripper,
	)

	t.Run("User agent gets set initially", func(t *testing.T) {
		expectedUserAgent = "unset/0"
		if err := client.QueryWithGitHubAppsSupport(context.Background(), struct{}{}, nil, ""); err != nil {
			t.Error(err)
		}
	})

	t.Run("ForPlugin changes the user agent accordingly", func(t *testing.T) {
		client := client.ForPlugin("test-plugin")
		expectedUserAgent = "unset.test-plugin/0"
		if err := client.QueryWithGitHubAppsSupport(context.Background(), struct{}{}, nil, ""); err != nil {
			t.Error(err)
		}
	})

	t.Run("The ForPlugin call doesn't manipulate the original client", func(t *testing.T) {
		expectedUserAgent = "unset/0"
		if err := client.QueryWithGitHubAppsSupport(context.Background(), struct{}{}, nil, ""); err != nil {
			t.Error(err)
		}
	})

	t.Run("ForSubcomponent changes the user agent accordingly", func(t *testing.T) {
		client := client.ForSubcomponent("test-plugin")
		expectedUserAgent = "unset.test-plugin/0"
		if err := client.QueryWithGitHubAppsSupport(context.Background(), struct{}{}, nil, ""); err != nil {
			t.Error(err)
		}
	})

	t.Run("The ForSubcomponent call doesn't manipulate the original client", func(t *testing.T) {
		expectedUserAgent = "unset/0"
		if err := client.QueryWithGitHubAppsSupport(context.Background(), struct{}{}, nil, ""); err != nil {
			t.Error(err)
		}
	})
}

func TestGetDirectory(t *testing.T) {
	expectedContents := []DirectoryContent{
		{
			Type: "file",
			Name: "bar",
			Path: "foo/bar",
		},
		{
			Type: "dir",
			Name: "hello",
			Path: "foo/hello",
		},
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/contents/foo" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("Bad request query: %s", r.URL.RawQuery)
		}
		b, err := json.Marshal(&expectedContents)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if contents, err := c.GetDirectory("k8s", "kuber", "foo", ""); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(contents) != 2 {
		t.Errorf("Expected two contents, found %d: %v", len(contents), contents)
		return
	} else if !reflect.DeepEqual(contents, expectedContents) {
		t.Errorf("Wrong list of teams, expected: %v, got: %v", expectedContents, contents)
	}
}

func TestGetDirectoryRef(t *testing.T) {
	expectedContents := []DirectoryContent{
		{
			Type: "file",
			Name: "bar.go",
			Path: "foo/bar.go",
		},
		{
			Type: "dir",
			Name: "hello",
			Path: "foo/hello",
		},
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/contents/foo" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "ref=12345" {
			t.Errorf("Bad request query: %s", r.URL.RawQuery)
		}
		b, err := json.Marshal(&expectedContents)
		if err != nil {
			t.Fatalf("Didn't expect error: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if contents, err := c.GetDirectory("k8s", "kuber", "foo", "12345"); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	} else if len(contents) != 2 {
		t.Errorf("Expected two contents, found %d: %v", len(contents), contents)
		return
	} else if !reflect.DeepEqual(contents, expectedContents) {
		t.Errorf("Wrong list of teams, expected: %v, got: %v", expectedContents, contents)
	}
}

func TestCreatePullRequestReviewComment(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/repos/k8s/kuber/pulls/5/comments" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Could not read request body: %v", err)
		}
		var rc ReviewComment
		if err := json.Unmarshal(b, &rc); err != nil {
			t.Errorf("Could not unmarshal request: %v", err)
		} else if rc.Body != "hello" {
			t.Errorf("Wrong body: %s", rc.Body)
		}
		http.Error(w, "201 Created", http.StatusCreated)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if err := c.CreatePullRequestReviewComment("k8s", "kuber", 5, ReviewComment{Body: "hello"}); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}
