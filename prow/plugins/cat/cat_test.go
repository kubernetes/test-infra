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

package cat

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakeClowder string

var human = flag.Bool("human", false, "Enable to run additional manual tests")
var category = flag.String("category", "", "Request a particular category if set")

func (c fakeClowder) readCat(category string) (string, error) {
	if category == "error" {
		return "", errors.New(string(c))
	}
	return string(c), nil
}

func TestRealCat(t *testing.T) {
	if !*human {
		t.Skip("Real cats disabled for automation. Manual users can add --human [--category=foo]")
	}
	if cat, err := catURL.readCat(*category); err != nil {
		t.Errorf("Could not read cats from %s: %v", catURL, err)
	} else {
		fmt.Println(cat)
	}
}

func TestFormat(t *testing.T) {
	re := regexp.MustCompile(`\[!\[.+\]\(.+\)\]\(.+\)`)
	basicURL := "http://example.com"
	testcases := []struct {
		name string
		src  string
		img  string
		err  bool
	}{
		{
			name: "basically works",
			src:  basicURL,
			img:  basicURL,
			err:  false,
		},
		{
			name: "empty source",
			src:  "",
			img:  basicURL,
			err:  true,
		},
		{
			name: "empty image",
			src:  basicURL,
			img:  "",
			err:  true,
		},
		{
			name: "bad source",
			src:  "http://this is not a url",
			img:  basicURL,
			err:  true,
		},
		{
			name: "bad image",
			src:  basicURL,
			img:  "http://still a bad url",
			err:  true,
		},
	}
	for _, tc := range testcases {
		ret, err := catResult{
			Source: tc.src,
			Image:  tc.img,
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
	img := "http://localhost?kind=url"
	src := "http://localhost?kind=source_url"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<response><data><images><image><url>%s</url><source_url>%s</source_url></image></images></data></response>`, img, src)
	}))
	defer ts.Close()
	fc := &fakegithub.FakeClient{
		IssueComments: make(map[int][]github.IssueComment),
	}

	comment := "/meow"

	e := &github.GenericCommentEvent{
		Action:     github.GenericCommentActionCreated,
		Body:       comment,
		Number:     5,
		IssueState: "open",
	}
	if err := handle(fc, logrus.WithField("plugin", pluginName), e, realClowder(ts.URL)); err != nil {
		t.Errorf("didn't expect error: %v", err)
		return
	}
	if len(fc.IssueComments[5]) != 1 {
		t.Error("should have commented.")
		return
	}
	if c := fc.IssueComments[5][0]; !strings.Contains(c.Body, img) {
		t.Errorf("missing image url: %s from comment: %v", img, c)
	} else if !strings.Contains(c.Body, src) {
		t.Errorf("missing source url: %s from comment: %v", src, c)
	}

}

// Small, unit tests
func TestCats(t *testing.T) {
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
			body:          "/meow",
			shouldComment: false,
			shouldError:   false,
		},
		{
			name:          "leave cat on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/meow",
			pr:            true,
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave cat on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/meow",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave cat on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/meow \r",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "categorical cat",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/meow clothes",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "bad cat",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/meow error",
			shouldComment: false,
			shouldError:   true,
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
		err := handle(fc, logrus.WithField("plugin", pluginName), e, fakeClowder("tubbs"))
		if !tc.shouldError && err != nil {
			t.Errorf("For case %s, didn't expect error: %v", tc.name, err)
			continue
		} else if tc.shouldError && err == nil {
			t.Errorf("For case %s, expected an error to occur", tc.name)
			continue
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("For case %s, should have commented.", tc.name)
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("For case %s, should not have commented.", tc.name)
		}
	}
}
