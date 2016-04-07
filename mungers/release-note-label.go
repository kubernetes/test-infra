/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package mungers

import (
	"fmt"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/spf13/cobra"
)

const (
	releaseNoteLabeler = "release-note-label"

	releaseNoteLabelNeeded    = "release-note-label-needed"
	releaseNote               = "release-note"
	releaseNoteNone           = "release-note-none"
	releaseNoteActionRequired = "release-note-action-required"
)

// ReleaseNoteLabel will remove the LGTM label from an PR which has not
// set one of the appropriete 'release-note-*' labels.
type ReleaseNoteLabel struct {
	config *github.Config
}

func init() {
	RegisterMungerOrDie(&ReleaseNoteLabel{})
}

// Name is the name usable in --pr-mungers
func (r *ReleaseNoteLabel) Name() string { return releaseNoteLabeler }

// RequiredFeatures is a slice of 'features' that must be provided
func (r *ReleaseNoteLabel) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (r *ReleaseNoteLabel) Initialize(config *github.Config, features *features.Features) error {
	r.config = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (r *ReleaseNoteLabel) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (r *ReleaseNoteLabel) AddFlags(cmd *cobra.Command, config *github.Config) {}

func (r *ReleaseNoteLabel) prMustFollowRelNoteProcess(obj *github.MungeObject) bool {
	if obj.IsForBranch("master") {
		return true
	}

	parents := getCherrypickParentPRs(obj, r.config)
	// if it has no parents it needs to follow the release note process
	if len(parents) == 0 {
		return true
	}

	for _, parent := range parents {
		// If the parent didn't set a release note, the CP must
		if !parent.HasLabel(releaseNote) && !parent.HasLabel(releaseNoteActionRequired) {
			return true
		}
	}
	// All of the parents set the releaseNote or releaseNoteActionRequired label, so
	// this cherrypick PR needs to do nothing.
	return false
}

// Munge is the workhorse the will actually make updates to the PR
func (r *ReleaseNoteLabel) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !r.prMustFollowRelNoteProcess(obj) {
		return
	}

	if obj.HasLabel(releaseNote) || obj.HasLabel(releaseNoteActionRequired) || obj.HasLabel(releaseNoteNone) {
		if obj.HasLabel(releaseNoteLabelNeeded) {
			obj.RemoveLabel(releaseNoteLabelNeeded)
		}
		return
	}

	if !obj.HasLabel(releaseNoteLabelNeeded) {
		obj.AddLabel(releaseNoteLabelNeeded)
	}

	if !obj.HasLabel("lgtm") {
		return
	}

	msgFmt := `Removing LGTM because the release note process has not been followed.
One of the following labels is required %q, %q, or %q
Please see: https://github.com/kubernetes/kubernetes/blob/master/docs/devel/pull-requests.md#release-notes`
	msg := fmt.Sprintf(msgFmt, releaseNote, releaseNoteNone, releaseNoteActionRequired)
	obj.WriteComment(msg)
	obj.RemoveLabel("lgtm")
}
