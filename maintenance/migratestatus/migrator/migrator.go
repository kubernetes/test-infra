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

package migrator

import (
	"fmt"

	"k8s.io/test-infra/ghclient"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	stateAny = "ANY_STATE"
	stateDNE = "DOES_NOT_EXIST"
)

// contextCondition is a struct that describes a condition about the state or existence of a context.
type contextCondition struct {
	// context is the status context that this condition applies to.
	context string
	// state is the status state that the condition accepts, or one of the special values "ANY_STATE"
	// and "DOES_NOT_EXIST".
	state string
}

// Mode is a struct that describes the behavior of a status migration. The behavior is described as
// a list of conditions and a function that determines the actions to be taken when the conditions
// are met.
type Mode struct {
	conditions []*contextCondition
	// actions returns the status updates to make based on the current statuses and the sha.
	// When actions is called, the Mode may assume that it's conditions are met.
	actions func(statuses []github.RepoStatus, sha string) []*github.RepoStatus
}

// MoveMode creates a mode that both copies and retires.
// The mode creates a new context on every PR with the old context but not the new one, setting the
// state of the new context to that of the old context before retiring the old context.
func MoveMode(origContext, newContext string) *Mode {
	dup := copyAction(origContext, newContext)
	dep := retireAction(origContext, newContext)

	return &Mode{
		conditions: []*contextCondition{
			{context: origContext, state: stateAny},
			{context: newContext, state: stateDNE},
		},
		actions: func(statuses []github.RepoStatus, sha string) []*github.RepoStatus {
			return append(dup(statuses, sha), dep(statuses, sha)...)
		},
	}
}

// CopyMode makes a mode that creates a new context in every PR that has the old context, but not the new one.
// The state, description and target URL of the new context are made the same as those of the old context.
func CopyMode(origContext, newContext string) *Mode {
	return &Mode{
		conditions: []*contextCondition{
			{context: origContext, state: stateAny},
			{context: newContext, state: stateDNE},
		},
		actions: copyAction(origContext, newContext),
	}
}

// RetireMode creates a mode that retires an old context on all PRs.
// If newContext is the empty string, origContext is retired without replacement. Its state is set to
// 'success' and its description is set to indicate that the context is retired.
// If newContext is not the empty string it is considered the replacement of origContext. This means
// that only PRs that have the newContext in addition to the origContext will be considered and the
// description of the retired context will indicate that it was replaced by newContext.
func RetireMode(origContext, newContext string) *Mode {
	conditions := []*contextCondition{{context: origContext, state: stateAny}}
	if newContext != "" {
		conditions = append(conditions, &contextCondition{context: newContext, state: stateAny})
	}
	return &Mode{
		conditions: conditions,
		actions:    retireAction(origContext, newContext),
	}
}

// copyAction creates a function that returns a copy action.
// Specifically the returned function returns a RepoStatus that will create a status for newContext
// with state set to the state of origContext.
func copyAction(origContext, newContext string) func(statuses []github.RepoStatus, sha string) []*github.RepoStatus {
	return func(statuses []github.RepoStatus, sha string) []*github.RepoStatus {
		var oldStatus *github.RepoStatus
		for _, status := range statuses {
			if status.Context != nil && *status.Context == origContext {
				oldStatus = &status
				break
			}
		}
		if oldStatus == nil {
			// This means the conditions were not met! Should never have called this function, but it is a recoverable error.
			glog.Errorf("failed to find original context in status list thus conditions for this duplicate action were not met. This should never happen!")
			return nil
		}
		return []*github.RepoStatus{
			{
				Context:     &newContext,
				State:       oldStatus.State,
				TargetURL:   oldStatus.TargetURL,
				Description: oldStatus.Description,
			},
		}
	}
}

// retireAction creates a function that returns a retire action.
// Specifically the returned function returns a RepoStatus that will update the origContext status
// to 'success' and set it's description to mark it as retired and replaced by newContext.
func retireAction(origContext, newContext string) func(statuses []github.RepoStatus, sha string) []*github.RepoStatus {
	stateSuccess := "success"
	var desc string
	if newContext == "" {
		desc = fmt.Sprint("Context retired without replacement.")
	} else {
		desc = fmt.Sprintf("Context retired. Status moved to \"%s\".", newContext)
	}
	return func(statuses []github.RepoStatus, sha string) []*github.RepoStatus {
		return []*github.RepoStatus{
			{
				Context:     &origContext,
				State:       &stateSuccess,
				TargetURL:   nil,
				Description: &desc,
			},
		}
	}
}

// processStatuses checks the mode against the combined status of a PR and emits the actions to take.
func (m Mode) processStatuses(combStatus *github.CombinedStatus) []*github.RepoStatus {
	var sha string
	if combStatus.SHA != nil {
		sha = *combStatus.SHA
	}

	for _, cond := range m.conditions {
		var match *github.RepoStatus
		match = nil
		for _, status := range combStatus.Statuses {
			if status.Context == nil {
				glog.Errorf("a status context for SHA ref '%s' had a nil Context field.", sha)
				continue
			}
			if *status.Context == cond.context {
				match = &status
				break
			}
		}

		switch cond.state {
		case stateDNE:
			if match != nil {
				return nil
			}
		case stateAny:
			if match == nil {
				return nil
			}
		default:
			// Looking for a specific state in this case.
			if match == nil {
				// Did not find the context.
				return nil
			}
			if match.State == nil {
				glog.Errorf("context '%s' of SHA ref '%s' has a nil state.", cond.context, sha)
				return nil
			}
			if *match.State != cond.state {
				// Context had a different state than what the condition requires.
				return nil
			}
		}
	}
	return m.actions(combStatus.Statuses, sha)
}

// Migrator will search github for PRs with a given context and migrate/retire/move them.
type Migrator struct {
	org             string
	repo            string
	continueOnError bool

	client *ghclient.Client
	Mode
}

// New creates a new migrator with specified options.
//
// If dryRun is true it will only perform GET requests that do not change github.
func New(mode Mode, token, org, repo string, dryRun, continueOnError bool) *Migrator {
	return &Migrator{
		org:             org,
		repo:            repo,
		continueOnError: continueOnError,
		client:          ghclient.NewClient(token, dryRun),
		Mode:            mode,
	}
}

func (m *Migrator) processPR(pr *github.PullRequest) error {
	if pr == nil {
		return fmt.Errorf("migrator cannot process a nil PullRequest")
	}
	if pr.Head == nil {
		return fmt.Errorf("migrator cannot process a PullRequest with a nil 'Head' field")
	}
	if pr.Head.SHA == nil {
		return fmt.Errorf("migrator cannot process a PullRequest with a nil 'Head.SHA' field")
	}

	combined, err := m.client.GetCombinedStatus(m.org, m.repo, *pr.Head.SHA)
	if err != nil {
		return err
	}
	actions := m.processStatuses(combined)

	for _, action := range actions {
		if _, err = m.client.CreateStatus(m.org, m.repo, *pr.Head.SHA, action); err != nil {
			return err
		}
	}
	return nil
}

// Migrate will retire/migrate/copy statuses for all matching PRs.
func (m *Migrator) Migrate(prOptions *github.PullRequestListOptions) error {
	return m.client.ForEachPR(m.org, m.repo, prOptions, m.continueOnError, m.processPR)
}
