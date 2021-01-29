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

package goose

import (
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

type fakeGaggle string

var human = flag.Bool("human", false, "Enable to run additional manual tests")
var keyPath = flag.String("key-path", "", "Path to api key if set")

func (g fakeGaggle) readGoose() (string, error) {
	return fmt.Sprintf("\n![fake goose image](%s)", g), nil
}

func TestRealGoose(t *testing.T) {
	if !*human {
		t.Skip("Real geese disabled for automation. Manual users can add --human")
	}
	if *keyPath != "" {
		honk.setKey(*keyPath, logrus.WithField("plugin", pluginName))
	}

	if goose, err := honk.readGoose(); err != nil {
		t.Errorf("Could not read geese from %#v: %v", honk, err)
	} else {
		fmt.Println(goose)
	}
}

func TestUrl(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		key     string
		require []string
		deny    []string
	}{
		{
			name: "only url",
			url:  "http://foo",
		},
		{
			name:    "key",
			url:     "http://foo",
			key:     "blah",
			require: []string{"client_id=blah"},
		},
	}

	for _, tc := range cases {
		rg := realGaggle{
			url: tc.url,
			key: tc.key,
		}
		url := rg.URL()
		for _, r := range tc.require {
			if !strings.Contains(url, r) {
				t.Errorf("%s: %s does not contain %s", tc.name, url, r)
			}
		}
		for _, d := range tc.deny {
			if strings.Contains(url, d) {
				t.Errorf("%s: %s contained unexpected %s", tc.name, url, d)
			}
		}
	}
}

func TestFormat(t *testing.T) {
	re := regexp.MustCompile(`!\[.+\]\(.+\)`)
	basicURL := "http://example.com"
	testcases := []struct {
		name string
		img  string
		err  bool
	}{
		{
			name: "basically works",
			img:  basicURL,
			err:  false,
		},
		{
			name: "empty image",
			img:  "",
			err:  true,
		},
		{
			name: "bad image",
			img:  "http://still a bad url",
			err:  true,
		},
	}
	for _, tc := range testcases {
		ret, err := gooseResult{
			Images: imageSet{
				Small: tc.img,
			},
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
	// create test cases for handling content length of images
	contentLength := make(map[string]string)
	contentLength["/goose.jpg"] = "717987"
	contentLength["/biggoose.jpg"] = "12647753"

	// fake server for images
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s, ok := contentLength[r.URL.Path]; ok {
			body := "binary image"
			w.Header().Set("Content-Length", s)
			io.WriteString(w, body)
		} else {
			t.Errorf("Cannot find content length for %s", r.URL.Path)
		}
	}))
	defer ts2.Close()

	// create test cases for handling http responses
	img := ts2.URL + "/goose.jpg"
	bigimg := ts2.URL + "/biggoose.jpg"
	validResponse := fmt.Sprintf(`{"id":"valid","urls":{"small":"%s"}}`, img)
	var testcases = []struct {
		name     string
		path     string
		response string
		valid    bool
		code     int
	}{
		{
			name:     "valid",
			path:     "/valid",
			response: validResponse,
			valid:    true,
		},
		{
			name:     "image too big",
			path:     "/too-big",
			response: fmt.Sprintf(`{"id":"valid","urls":{"small":"%s"}}`, bigimg),
		},
		{
			name: "return-406",
			path: "/return-406",
			code: 406,
			response: `
<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>406 Not Acceptable</title>
</head><body>
<h1>Not Acceptable</h1>
<p>An appropriate representation of the requested resource /api/images/get could not be found on this server.</p>
Available variants:
<ul>
<li><a href="get.php">get.php</a> , type x-mapp-php5</li>
</ul>
</body></html>`,
		},
		{
			name:     "no-geese-in-json",
			path:     "/no-geese-in-json",
			response: "[]",
		},
		{
			name:     "no-image-in-json",
			path:     "/no-image-in-json",
			response: "[{}]",
		},
	}

	// fake server for image urls
	pathToResponse := make(map[string]string)
	for _, testcase := range testcases {
		pathToResponse[testcase.path] = testcase.response
	}
	codes := make(map[string]int)
	for _, tc := range testcases {
		codes[tc.path] = tc.code
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := codes[r.URL.Path]
		if code > 0 {
			w.WriteHeader(code)
		}
		if r, ok := pathToResponse[r.URL.Path]; ok {
			io.WriteString(w, r)
		} else {
			io.WriteString(w, validResponse)
		}
	}))
	defer ts.Close()

	// github fake client
	fc := fakegithub.NewFakeClient()
	fc.IssueComments = make(map[int][]github.IssueComment)

	// run test for each case
	for _, testcase := range testcases {
		fakehonk := &realGaggle{url: ts.URL + testcase.path}
		goose, err := fakehonk.readGoose()
		if testcase.valid && err != nil {
			t.Errorf("For case %s, didn't expect error: %v", testcase.name, err)
		} else if !testcase.valid && err == nil {
			t.Errorf("For case %s, expected error, received goose: %s", testcase.name, goose)
		} else if testcase.valid && goose == "" {
			t.Errorf("For case %s, got an empty goose", testcase.name)
		}
	}

	// fully test handling a comment
	comment := "/honk"

	e := &github.GenericCommentEvent{
		Action:     github.GenericCommentActionCreated,
		Body:       comment,
		Number:     5,
		IssueState: "open",
	}
	if err := handle(fc, logrus.WithField("plugin", pluginName), e, &realGaggle{url: ts.URL + "/?format=json"}, func() {}); err != nil {
		t.Errorf("didn't expect error: %v", err)
		return
	}
	if len(fc.IssueComments[5]) != 1 {
		t.Error("should have commented.")
		return
	}
	if c := fc.IssueComments[5][0]; !strings.Contains(c.Body, img) {
		t.Errorf("missing image url: %s from comment: %v", img, c)
	}

}

// Small, unit tests
func TestGeese(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		body          string
		state         string
		pr            bool
		shouldComment bool
		shouldError   bool
	}{
		{
			name:          "ignore edited comment",
			state:         "open",
			action:        github.GenericCommentActionEdited,
			body:          "/honk",
			shouldComment: false,
			shouldError:   false,
		},
		{
			name:          "leave goose on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/honk",
			pr:            true,
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave goose on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/honk",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave goose on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/honk \r",
			shouldComment: true,
			shouldError:   false,
		},
	}
	for _, tc := range testcases {
		fc := fakegithub.NewFakeClient()
		fc.IssueComments = make(map[int][]github.IssueComment)
		e := &github.GenericCommentEvent{
			Action:     tc.action,
			Body:       tc.body,
			Number:     5,
			IssueState: tc.state,
			IsPR:       tc.pr,
		}
		err := handle(fc, logrus.WithField("plugin", pluginName), e, fakeGaggle("thegoose"), func() {})
		if !tc.shouldError && err != nil {
			t.Errorf("%s: didn't expect error: %v", tc.name, err)
			continue
		} else if tc.shouldError && err == nil {
			t.Errorf("%s: expected an error to occur", tc.name)
			continue
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("%s: should have commented.", tc.name)
		} else if tc.shouldComment {
			shouldImage := !tc.shouldError
			body := fc.IssueComments[5][0].Body
			hasImage := strings.Contains(body, "![")
			if hasImage && !shouldImage {
				t.Errorf("%s: unexpected image in %s", tc.name, body)
			} else if !hasImage && shouldImage {
				t.Errorf("%s: no image in %s", tc.name, body)
			}
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("%s: should not have commented.", tc.name)
		}
	}
}
