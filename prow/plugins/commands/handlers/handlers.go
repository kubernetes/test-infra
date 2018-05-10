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

package handlers

import (
	"fmt"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins/commands"
)

type labelClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
}

// EnsureLabel adds the specified label if it is not already present.
func EnsureLabel(label string) commands.MatchHandler {
	return func(ctx *commands.Context) error {
		return doEnsureLabel(ctx.Client.GitHubClient, ctx.Event, label)
	}
}

func doEnsureLabel(client labelClient, e *github.GenericCommentEvent, label string) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	currentLabels, err := client.GetIssueLabels(org, repo, number)
	if err != nil {
		return fmt.Errorf("error listing labels: %v", err)
	}
	if github.HasLabel(label, currentLabels) {
		return nil
	}
	if err := client.AddLabel(org, repo, number, label); err != nil {
		return fmt.Errorf("error adding label %q: %v", label, err)
	}
	return nil
}

// RemoveLabel removes the specified label.
// We don't check if the label is present to save API tokens. If it is not
// present this is a no-op.
// Note: This may actually use more tokens rather than less if the github cache
// proxy is in use and the label is usually not present. /shrug
func RemoveLabel(label string) commands.MatchHandler {
	return func(ctx *commands.Context) error {
		return doRemoveLabel(ctx.Client.GitHubClient, ctx.Event, label)
	}
}

func doRemoveLabel(client labelClient, e *github.GenericCommentEvent, label string) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	if err := client.RemoveLabel(org, repo, number, label); err != nil {
		if _, ok := err.(*github.LabelNotFound); !ok {
			return fmt.Errorf("error removing label %q: %v", label, err)
		}
	}
	return nil
}

/*
	Additional possible handlers:

	func Assign(ctx *Context) error { ... }
	func RequestReview(ctx *Context) error { ... }

	- Post to slack
	- Add emoji reaction
	- Set milestone
*/
