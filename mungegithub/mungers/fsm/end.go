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

	"fmt"
)

const (
	// EndState is the name of the end state.
	endState = "EndState"
)

// End is the end state when we can't proceed.
type End struct{}

var _ State = &End{}

// Initialize can be used to set up initialization.
func (e *End) Initialize(obj *github.MungeObject) error {
	return nil
}

// Process does the necessary processing to compute whether to stay in
// this state, or proceed to the next.
func (e *End) Process(obj *github.MungeObject) (State, error) {
	return &End{}, fmt.Errorf("Cannot process end state.")
}

// Name is the name of the state machine's state.
func (e *End) Name() string {
	return endState
}
