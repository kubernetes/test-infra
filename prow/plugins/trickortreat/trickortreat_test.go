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

package trickortreat

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakeClowder struct {
	errStr string
}

func (c *fakeClowder) readImage() (string, error) {
	if c.errStr != "" {
		return "", errors.New(c.errStr)
	}
	return fmt.Sprintf("![fake candy image](%s)", c), nil
}

func TestImages(t *testing.T) {
	for _, imgURL := range candiesImgs {
		t.Run(imgURL, func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 3; i++ {
				toobig, err := github.ImageTooBig(imgURL)
				if err != nil {
					t.Errorf("Failed reading image: %v", err)
					continue
				}
				if toobig {
					t.Errorf("Image %q too big", imgURL)
				}
				break
			}
		})
	}
}

func TestReadImage(t *testing.T) {
	if img, err := trickortreat.readImage(); err != nil {
		t.Errorf("Could not read candies from %#v: %v", trickortreat, err)
	} else {
		t.Log(img)
	}
}

// Small, unit tests
func TestAll(t *testing.T) {
	var testcases = []struct {
		name             string
		action           github.GenericCommentEventAction
		body             string
		state            string
		pr               bool
		readImgErrString string
		shouldComment    bool
		shouldError      bool
	}{
		{
			name:             "failed reading image",
			state:            "open",
			action:           github.GenericCommentActionCreated,
			body:             "/trick-or-treat",
			readImgErrString: "failed",
			shouldComment:    false,
			shouldError:      true,
		},
		{
			name:          "ignore edited comment",
			state:         "open",
			action:        github.GenericCommentActionEdited,
			body:          "/trick-or-treat",
			shouldComment: false,
			shouldError:   false,
		},
		{
			name:          "leave candy on pr",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/trick-or-treat",
			pr:            true,
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave candy on issue",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/trick-or-treat",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "leave candy on issue, trailing space",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/trick-or-treat \r",
			shouldComment: true,
			shouldError:   false,
		},
		{
			name:          "Trailing random strings",
			state:         "open",
			action:        github.GenericCommentActionCreated,
			body:          "/trick-or-treat clothes",
			shouldComment: true,
			shouldError:   false,
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
		err := handle(fc, logrus.WithField("plugin", pluginName), e, &fakeClowder{tc.readImgErrString})
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
