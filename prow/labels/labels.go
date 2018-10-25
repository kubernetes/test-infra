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
	LGTM            = "lgtm"
	BlockedPaths    = "do-not-merge/blocked-paths"
	LifecycleActive = "lifecycle/active"
	LifecycleFrozen = "lifecycle/frozen"
	LifecycleStale  = "lifecycle/stale"
	LifecycleRotten = "lifecycle/rotten"
	ClaYes          = "cncf-cla: yes"
	ClaNo           = "cncf-cla: no"
	Approved        = "approved"
	InvalidOwners   = "do-not-merge/invalid-owners-file"
	CpUnapproved    = "do-not-merge/cherry-pick-not-approved"
	CpApproved      = "cherry-pick-approved"
	WorkInProgress  = "do-not-merge/work-in-progress"
	Hold            = "do-not-merge/hold"
	Shrug           = "¯\\_(ツ)_/¯"
	NeedsSig        = "needs-sig"
	Bug             = "kind/bug"
	Help            = "help wanted"
	GoodFirstIssue  = "good first issue"
	NeedsRebase     = "needs-rebase"
)
