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

package labels

// labels for github plugins
const (
	Approved                    = "approved"
	BlockedPaths                = "do-not-merge/blocked-paths"
	Bug                         = "kind/bug"
	BugzillaSeverityUrgent      = "bugzilla/severity-urgent"
	BugzillaSeverityHigh        = "bugzilla/severity-high"
	BugzillaSeverityMed         = "bugzilla/severity-medium"
	BugzillaSeverityLow         = "bugzilla/severity-low"
	BugzillaSeverityUnspecified = "bugzilla/severity-unspecified"
	ClaNo                       = "cncf-cla: no"
	ClaYes                      = "cncf-cla: yes"
	CpApproved                  = "cherry-pick-approved"
	CpUnapproved                = "do-not-merge/cherry-pick-not-approved"
	GoodFirstIssue              = "good first issue"
	Help                        = "help wanted"
	Hold                        = "do-not-merge/hold"
	InvalidOwners               = "do-not-merge/invalid-owners-file"
	InvalidBug                  = "bugzilla/invalid-bug"
	LGTM                        = "lgtm"
	LifecycleActive             = "lifecycle/active"
	LifecycleFrozen             = "lifecycle/frozen"
	LifecycleRotten             = "lifecycle/rotten"
	LifecycleStale              = "lifecycle/stale"
	MergeCommits                = "do-not-merge/contains-merge-commits"
	NeedsOkToTest               = "needs-ok-to-test"
	NeedsRebase                 = "needs-rebase"
	NeedsSig                    = "needs-sig"
	OkToTest                    = "ok-to-test"
	Shrug                       = "¯\\_(ツ)_/¯"
	TriageAccepted              = "triage/accepted"
	WorkInProgress              = "do-not-merge/work-in-progress"
	ValidBug                    = "bugzilla/valid-bug"
)
