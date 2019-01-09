/*
Copyright 2018 The Kubernetes Authors.

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

package pony

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakeHerd string

var human = flag.Bool("human", false, "Enable to run additional manual tests")
var ponyFlag = flag.String("pony", "", "Request a particular pony if set")

func (c fakeHerd) readPony(tags string) (string, error) {
	if tags != "" {
		return tags, nil
	}
	return string(c), nil
}

func TestRealPony(t *testing.T) {
	if !*human {
		t.Skip("Real ponies disabled for automation. Manual users can add --human [--category=foo]")
	}
	if pony, err := ponyURL.readPony(*ponyFlag); err != nil {
		t.Errorf("Could not read pony from %s: %v", ponyURL, err)
	} else {
		fmt.Println(pony)
	}
}

func TestFormat(t *testing.T) {
	result := formatURLs("http://example.com/small", "http://example.com/full")
	expected := "[![pony image](http://example.com/small)](http://example.com/full)"
	if result != expected {
		t.Errorf("Expected %q, but got %q", expected, result)
	}
}

// Medium integration test (depends on ability to open a TCP port)
func TestHttpResponse(t *testing.T) {

	// create test cases for handling content length of images
	contentLength := make(map[string]string)
	contentLength["/pony.jpg"] = "717987"
	contentLength["/horse.png"] = "12647753"

	// fake server for images
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/full" {
			t.Errorf("Requested full-size image instead of small image.")
			http.NotFound(w, r)
			return
		}
		if s, ok := contentLength[r.URL.Path]; ok {
			body := "binary image"
			w.Header().Set("Content-Length", s)
			io.WriteString(w, body)
		} else {
			t.Errorf("Cannot find content length for %s", r.URL.Path)
		}
	}))
	defer ts2.Close()

	// setup a stock valid request
	url := ts2.URL + "/pony.jpg"
	b, err := json.Marshal(&ponyResult{
		Pony: ponyResultPony{
			Representations: ponyRepresentations{
				Small: ts2.URL + "/pony.jpg",
				Full:  ts2.URL + "/full",
			},
		},
	})
	if err != nil {
		t.Errorf("Failed to encode test data: %v", err)
	}

	// create test cases for handling http responses
	validResponse := string(b)

	type testcase struct {
		name      string
		comment   string
		path      string
		response  string
		expected  string
		expectTag string
		isValid   bool
		noPony    bool
	}

	var testcases = []testcase{
		{
			name:     "valid",
			comment:  "/pony",
			path:     "/valid",
			response: validResponse,
			expected: url,
			isValid:  true,
		},
		{
			name:    "no pony found",
			comment: "/pony",
			path:    "/404",
			noPony:  true,
			isValid: false,
		},
		{
			name:     "invalid JSON",
			comment:  "/pony",
			path:     "/bad-json",
			response: `{"bad-blob": "not-a-url"`,
			isValid:  false,
		},
		{
			name:     "image too big",
			comment:  "/pony",
			path:     "/too-big",
			response: fmt.Sprintf(`{"pony":{"representations": {"small": "%s/horse.png", "full": "%s/full"}}}`, ts2.URL, ts2.URL),
			isValid:  false,
		},
		{
			name:      "has tag",
			comment:   "/pony peach hack",
			path:      "/peach",
			isValid:   true,
			expectTag: "peach hack",
			response:  validResponse,
		},
	}

	// fake server for image urls
	pathToTestCase := make(map[string]*testcase)
	for _, testcase := range testcases {
		tc := testcase
		pathToTestCase[testcase.path] = &tc
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc, ok := pathToTestCase[r.URL.Path]; ok {
			if tc.noPony {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query().Get("q")
			if strings.HasSuffix(q, ",") {
				t.Errorf("Expected query without trailing comma: %q", q)
			}
			if tc.expectTag != "" && !strings.HasSuffix(q, ", "+tc.expectTag) {
				t.Errorf("Expected tag %q, but didn't find it in %q", tc.expectTag, q)
			}
			io.WriteString(w, tc.response)
		} else {
			io.WriteString(w, validResponse)
		}
	}))
	defer ts.Close()

	// run test for each case
	for _, testcase := range testcases {
		pony, err := realHerd(ts.URL + testcase.path).readPony(testcase.expectTag)
		if testcase.isValid && err != nil {
			t.Errorf("For case %s, didn't expect error: %v", testcase.name, err)
		} else if !testcase.isValid && err == nil {
			t.Errorf("For case %s, expected error, received pony: %s", testcase.name, pony)
		}

		if !testcase.isValid {
			continue
		}

		// github fake client
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}

		// fully test handling a comment
		e := &github.GenericCommentEvent{
			Action:     github.GenericCommentActionCreated,
			Body:       testcase.comment,
			Number:     5,
			IssueState: "open",
		}
		err = handle(fc, logrus.WithField("plugin", pluginName), e, realHerd(ts.URL+testcase.path))
		if err != nil {
			t.Errorf("tc %s: For comment %s, didn't expect error: %v", testcase.name, testcase.comment, err)
		}

		if len(fc.IssueComments[5]) != 1 {
			t.Errorf("tc %s: should have commented", testcase.name)
		}
		if c := fc.IssueComments[5][0]; !strings.Contains(c.Body, testcase.expected) {
			t.Errorf("tc %s: missing image url: %s from comment: %v", testcase.name, testcase.expected, c.Body)
		}
	}
}

// Small, unit tests
func TestPonies(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		body          string
		state         string
		pr            bool
		shouldComment bool
	}{
		{
			name:          "ignore edited comment",
			state:         "open",
			action:        github.GenericCommentActionEdited,
			body:          "/pony",
			shouldComment: false,
		},
		{
			name:          "leave pony on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/pony",
			pr:            true,
			shouldComment: true,
		},
		{
			name:          "leave pony on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/pony",
			shouldComment: true,
		},
		{
			name:          "leave pony on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/pony \r",
			shouldComment: true,
		},
		{
			name:          "leave pony on issue, tag specified",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/pony Twilight Sparkle",
			shouldComment: true,
		},
		{
			name:          "leave pony on issue, tag specified, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/pony Twilight Sparkle \r",
			shouldComment: true,
		},
		{
			name:          "don't leave cats or dogs",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/woof\n/meow",
			shouldComment: false,
		},
		{
			name:          "do nothing in the middle of a line",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "did you know that /pony makes ponies happen?",
			shouldComment: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.GenericCommentEvent{
			Action:     tc.action,
			Body:       tc.body,
			Number:     5,
			IssueState: tc.state,
			IsPR:       tc.pr,
		}
		err := handle(fc, logrus.WithField("plugin", pluginName), e, fakeHerd("pone"))
		if err != nil {
			t.Errorf("For case %s, didn't expect error: %v", tc.name, err)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("For case %s, should have commented.", tc.name)
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("For case %s, should not have commented.", tc.name)
		}
	}
}
