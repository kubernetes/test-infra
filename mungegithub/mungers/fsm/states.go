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
	"k8s.io/contrib/mungegithub/github"
)

const (
	labelPreReview     = "state/prereview"
	labelNeedsReview   = "state/needs-review"
	labelChangesNeeded = "state/needs-changes"
	labelNeedsApproval = "state/needs-approval"
)

// ComputeState can be used to compute the state of each PR.
func ComputeState(obj *github.MungeObject) error {
	if !obj.IsPR() {
		return nil
	}

	// Every PR starts in the pre-review state.
	var currentState State = &PreReview{}
	for currentState.Name() != endState {
		var err error
		currentState, err = currentState.Process(obj)
		if err != nil {
			return err
		}
	}
	return nil
}

// State is the interface implemented by all states.
type State interface {
	Process(obj *github.MungeObject) (State, error)
	Name() string
}
