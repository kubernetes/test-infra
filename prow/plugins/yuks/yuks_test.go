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

package yuks

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakeJoke string

var human = flag.Bool("human", false, "Enable to run additional manual tests")

func (j fakeJoke) readJoke() (string, error) {
	return string(j), nil
}

func TestRealJoke(t *testing.T) {
	if !*human {
		t.Skip("Real jokes disabled for automation. Manual users can add --human")
	}
	if joke, err := jokeURL.readJoke(); err != nil {
		t.Errorf("Could not read joke from %s: %v", jokeURL, err)
	} else {
		fmt.Println(joke)
	}
}

// Medium integration test (depends on ability to open a TCP port)
func TestJokesMedium(t *testing.T) {
	j := "What do you get when you cross a joke with a rhetorical question?"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"joke": "%s"}`, j)
	}))
	defer ts.Close()
	fc := fakegithub.NewFakeClient()

	comment := "/joke"

	e := &github.GenericCommentEvent{
		Action:     github.GenericCommentActionCreated,
		Body:       comment,
		Number:     5,
		IssueState: "open",
	}
	if err := handle(fc, logrus.WithField("plugin", pluginName), e, realJoke(ts.URL)); err != nil {
		t.Errorf("didn't expect error: %v", err)
		return
	}
	if len(fc.IssueComments[5]) != 1 {
		t.Error("should have commented.")
		return
	}
	if c := fc.IssueComments[5][0]; !strings.Contains(c.Body, j) {
		t.Errorf("missing joke: %s from comment: %v", j, c)
	}
}

// Small, unit tests
func TestJokes(t *testing.T) {
	var testcases = []struct {
		name          string
		action        github.GenericCommentEventAction
		body          string
		state         string
		joke          fakeJoke
		pr            bool
		shouldComment bool
		shouldError   bool
	}{
		{
			name:          "ignore edited comment",
			state:         "open",
			action:        github.GenericCommentActionEdited,
			body:          "/joke",
			joke:          "this? that.",
			shouldComment: false,
			shouldError:   false,
		},
		{
			name:          "leave joke on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/joke",
			joke:          "this? that.",
			pr:            true,
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave joke on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/joke",
			joke:          "this? that.",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave joke on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/joke \r",
			joke:          "this? that.",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "reject bad joke chars",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/joke",
			joke:          "[hello](url)",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "empty joke",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/joke",
			joke:          "",
			shouldComment: false,
			shouldError:   true,
		},
	}
	for _, tc := range testcases {
		fc := fakegithub.NewFakeClient()
		e := &github.GenericCommentEvent{
			Action:     tc.action,
			Body:       tc.body,
			Number:     5,
			IssueState: tc.state,
			IsPR:       tc.pr,
		}
		err := handle(fc, logrus.WithField("plugin", pluginName), e, tc.joke)
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

// TestEscapeMarkdown tests escapeMarkdown
func TestEscapeMarkdown(t *testing.T) {
	var testcases = []struct {
		name     string
		joke     string
		expected string
	}{
		{
			name:     "simple characters, all allowed",
			joke:     "this? that.",
			expected: "this? that.",
		},
		{
			name:     "markdown url",
			joke:     "[hello](url)",
			expected: "&#91;hello&#93;&#40;url&#41;",
		},
		{
			name:     "bold move",
			joke:     "I made a <b>bold</b> move today: **move**",
			expected: "I made a &#60;b&#62;bold&#60;&#47;b&#62; move today&#58; &#42;&#42;move&#42;&#42;",
		},
		{
			name:     "helm symbol",
			joke:     "⎈",
			expected: "&#9096;",
		},
		{
			name:     "xss attempt",
			joke:     "<img src=404 onerror=alert('k8s')>",
			expected: "&#60;img src&#61;404 onerror&#61;alert&#40;'k8s'&#41;&#62;",
		},
		{
			name:     "empty joke",
			joke:     "",
			expected: "",
		},
		{
			name:     "longcat is long",
			joke:     "longcat is\n\n\n\n\n\n\nlong",
			expected: "longcat is&#10;&#10;&#10;&#10;&#10;&#10;&#10;long",
		},
	}
	for _, tc := range testcases {
		output := escapeMarkdown(tc.joke)
		if tc.expected != output {
			t.Errorf("For case %s, expected `%s` got `%s`", tc.name, tc.expected, output)
			continue
		}
	}
}
