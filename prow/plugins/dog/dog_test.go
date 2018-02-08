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

package dog

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakePack string

var human = flag.Bool("human", false, "Enable to run additional manual tests")

func (c fakePack) readDog() (string, error) {
	return string(c), nil
}

func TestRealDog(t *testing.T) {
	if !*human {
		t.Skip("Real dogs disabled for automation. Manual users can add --human [--category=foo]")
	}
	if dog, err := dogURL.readDog(); err != nil {
		t.Errorf("Could not read dog from %s: %v", dogURL, err)
	} else {
		fmt.Println(dog)
	}
}

func TestFormat(t *testing.T) {
	re := regexp.MustCompile(`\[!\[.+\]\(.+\)\]\(.+\)`)
	basicURL := "http://example.com"
	testcases := []struct {
		name string
		url  string
		err  bool
	}{
		{
			name: "basically works",
			url:  basicURL,
			err:  false,
		},
		{
			name: "empty url",
			url:  "",
			err:  true,
		},
		{
			name: "bad url",
			url:  "http://this is not a url",
			err:  true,
		},
	}
	for _, tc := range testcases {
		ret, err := dogResult{
			URL: tc.url,
		}.Format()
		switch {
		case tc.err:
			if err == nil {
				t.Errorf("%s: failed to raise an error", tc.name)
			}
		case err != nil:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case !re.MatchString(ret):
			t.Errorf("%s: bad return value: %s", tc.name, ret)
		}
	}
}

// Medium integration test (depends on ability to open a TCP port)
func TestHttpResponse(t *testing.T) {
	// setup a stock valid request
	url := "http://localhost/dog.jpg"
	b, err := json.Marshal(&dogResult{
		URL: url,
	})
	if err != nil {
		t.Errorf("Failed to encode test data: %v", err)
	}

	// create test cases for handling http responses
	validResponse := string(b)
	var testcases = []struct {
		name     string
		path     string
		response string
		isValid  bool
	}{
		{
			name:     "valid",
			path:     "/valid",
			response: validResponse,
			isValid:  true,
		},
		{
			name:     "invalid JSON",
			path:     "/bad-json",
			response: `{"bad-blob": "not-a-url"`,
			isValid:  false,
		},
		{
			name:     "invalid URL",
			path:     "/bad-url",
			response: `{"url": "not a url.."}`,
			isValid:  false,
		},
		{
			name:     "mp4 doggo unsupported :(",
			path:     "/mp4-doggo",
			response: `{"url": "http://example/doggo.mp4"}`,
			isValid:  false,
		},
	}

	// setup handler for test-cases
	pathToResponse := make(map[string]string)
	for _, testcase := range testcases {
		pathToResponse[testcase.path] = testcase.response
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r, ok := pathToResponse[r.URL.Path]; ok {
			io.WriteString(w, r)
		} else {
			io.WriteString(w, validResponse)
		}
	}))
	defer ts.Close()
	fc := &fakegithub.FakeClient{
		IssueComments: make(map[int][]github.IssueComment),
	}

	// run test for each case
	for _, testcase := range testcases {
		dog, err := realPack(ts.URL + testcase.path).readDog()
		if testcase.isValid && err != nil {
			t.Errorf("For case %s, didn't expect error: %v", testcase.name, err)
		} else if !testcase.isValid && err == nil {
			t.Errorf("For case %s, expected error, received dog: %s", testcase.name, dog)
		}
	}

	// fully test handling a comment
	comment := "/woof"
	e := &github.GenericCommentEvent{
		Action:     github.GenericCommentActionCreated,
		Body:       comment,
		Number:     5,
		IssueState: "open",
	}
	err = handle(fc, logrus.WithField("plugin", pluginName), e, realPack(ts.URL))
	if err != nil {
		t.Errorf("For comment /woof, didn't expect error: %v", err)
	}

	if len(fc.IssueComments[5]) != 1 {
		t.Error("should have commented.")
	}
	if c := fc.IssueComments[5][0]; !strings.Contains(c.Body, url) {
		t.Errorf("missing image url: %s from comment: %v", url, c)
	}
}

// Small, unit tests
func TestDogs(t *testing.T) {
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
			body:          "/woof",
			shouldComment: false,
		},
		{
			name:          "leave dog on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/woof",
			pr:            true,
			shouldComment: true,
		},
		{
			name:          "leave dog on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/woof",
			shouldComment: true,
		},
		{
			name:          "leave dog on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/woof \r",
			shouldComment: true,
		},
		{
			name:          "leave dog on issue, trailing /bark",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/bark",
			shouldComment: true,
		},
		{
			name:          "leave dog on issue, trailing /bark, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/bark \r",
			shouldComment: true,
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
		err := handle(fc, logrus.WithField("plugin", pluginName), e, fakePack("doge"))
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
