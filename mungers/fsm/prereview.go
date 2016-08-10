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

package fsm

import (
	githubhelper "k8s.io/contrib/mungegithub/github"
)

const (
	claYes   = "cla: yes"
	claNo    = "cla: no"
	claHuman = "cla: human-approved"

	releaseNote               = "release-note"
	releaseNoteActionRequired = "release-note-action-required"
	releaseNoteExperimental   = "release-note-experimental"
	releaseNoteLabelNeeded    = "release-note-none"
)

// PreReview is the state before the review starts.
type PreReview struct{}

var _ State = &PreReview{}

// Process does the necessary processing to compute whether to stay in
// this state, or proceed to the next.
func (p *PreReview) Process(obj *githubhelper.MungeObject) (State, error) {
	success := true
	if !p.checkCLA(obj) {
		success = false
	}

	if !p.checkReleaseNotes(obj) {
		success = false
	}

	if !p.checkAssignees(obj) {
		success = false
	}

	if success {
		if obj.HasLabel(labelPreReview) {
			obj.RemoveLabel(labelPreReview)
		}
		return &NeedsReview{}, nil
	}

	if !obj.HasLabel(labelPreReview) {
		obj.AddLabel(labelPreReview)
	}
	return &End{}, nil

}

func (p *PreReview) checkCLA(obj *githubhelper.MungeObject) bool {
	return obj.HasLabel(claYes) || obj.HasLabel(claHuman)
}

func (p *PreReview) checkReleaseNotes(obj *githubhelper.MungeObject) bool {
	return obj.HasLabel(releaseNote) || obj.HasLabel(releaseNoteActionRequired) || obj.HasLabel(releaseNoteExperimental) || obj.HasLabel(releaseNoteLabelNeeded)
}

func (p *PreReview) checkAssignees(obj *githubhelper.MungeObject) bool {
	return len(obj.Issue.Assignees) > 0
}

// Name is the name of the state machine's state.
func (p *PreReview) Name() string {
	return "PreReview"
}
