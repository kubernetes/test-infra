/*
Copyright 2020 The Kubernetes Authors.

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

package jira

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
)

// our client implements Client
var _ Client = &client{}

// our logger implements the leveledLogger inteface
var _ retryablehttp.LeveledLogger = &retryableHTTPLogrusWrapper{}

func TestJiraErrorStatusCode(t *testing.T) {
	err := &JiraError{
		StatusCode:    400,
		Body:          "something went wrong",
		OriginalError: errors.New("error: check response body for details"),
	}
	if code := JiraErrorStatusCode(err); code != 400 {
		t.Errorf("status code of unwrapped JiraError is %d; expected 400", code)
	}
	if code := JiraErrorStatusCode(fmt.Errorf("This is a wrapped error: %w", err)); code != 400 {
		t.Errorf("status code of wrapped JiraError is %d; expected 400", code)
	}
	if code := JiraErrorStatusCode(errors.New("This is not a jira error")); code != -1 {
		t.Errorf("status code of non-jira error is %d; expected -1", code)
	}
}

func TestJiraErrorBody(t *testing.T) {
	err := &JiraError{
		StatusCode:    400,
		Body:          "something went wrong",
		OriginalError: errors.New("error: check response body for details"),
	}
	if body := JiraErrorBody(err); body != "something went wrong" {
		t.Errorf("body of unwrapped JiraError is `%s`; expected `something went wrong`", body)
	}
	if body := JiraErrorBody(fmt.Errorf("This is a wrapped error: %w", err)); body != "something went wrong" {
		t.Errorf("body of wrapped JiraError is `%s`; expected `something went wrong`", body)
	}
	if body := JiraErrorBody(errors.New("This is not a jira error")); body != "" {
		t.Errorf("body of non-jira error is `%s`; expected empty string", body)
	}
}
