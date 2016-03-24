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
type ReleaseNoteLabel struct{}

func init() {
	RegisterMungerOrDie(ReleaseNoteLabel{})
}

// Name is the name usable in --pr-mungers
func (ReleaseNoteLabel) Name() string { return releaseNoteLabeler }

// RequiredFeatures is a slice of 'features' that must be provided
func (ReleaseNoteLabel) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (ReleaseNoteLabel) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (ReleaseNoteLabel) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (ReleaseNoteLabel) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (ReleaseNoteLabel) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
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
Please see: https://github.com/kubernetes/kubernetes/blob/master/docs/proposals/release-notes.md`
	msg := fmt.Sprintf(msgFmt, releaseNote, releaseNoteNone, releaseNoteActionRequired)
	obj.WriteComment(msg)
	obj.RemoveLabel("lgtm")
}
