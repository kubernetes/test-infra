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

	"github.com/golang/glog"
)

// ChangesNeeded is the state when the ball is in the author's court.
type ChangesNeeded struct{}

var _ State = &ChangesNeeded{}

// Process does the necessary processing to compute whether to stay in
// this state, or proceed to the next.
func (c *ChangesNeeded) Process(obj *github.MungeObject) (State, error) {
	if !obj.HasLabel(labelChangesNeeded) {
		obj.AddLabel(labelChangesNeeded)
		glog.Infof("PR #%v needs changes from author", *obj.Issue.Number)
	}
	return &End{}, nil
}

// Name is the name of the state machine's state.
func (c *ChangesNeeded) Name() string {
	return "ChangesNeeded"
}
