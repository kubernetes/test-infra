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

package plugins

import (
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
)

func TestFormatICResponse(t *testing.T) {
	ic := github.IssueComment{
		Body:    "Looks neat.\r\nI like it.\r\n",
		User:    github.User{Login: "ca"},
		HTMLURL: "happygoodsite.com",
	}
	s := "you are a nice person."
	out := FormatICResponse(ic, s)
	if !strings.HasPrefix(out, "@ca: you are a nice person.") {
		t.Errorf("Expected compliments to the comment author, got:\n%s", out)
	}
	if !strings.Contains(out, ">I like it.\r\n") {
		t.Errorf("Expected quotes, got:\n%s", out)
	}
}

func TestFormatResponseRaw(t *testing.T) {

	body := "Looks neat.\r\nI like it.\r\n"
	user := "ca"
	htmlURL := "happygoodsite.com"
	comment := "you are a nice person."

	out := FormatResponseRaw(body, htmlURL, user, comment)
	if !strings.HasPrefix(out, "@ca: you are a nice person.") {
		t.Errorf("Expected compliments to the comment author, got:\n%s", out)
	}
	if !strings.Contains(out, ">I like it.\r\n") {
		t.Errorf("Expected quotes, got:\n%s", out)
	}
}
