/*
Copyright 2019 The Kubernetes Authors.

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

package bugzilla

// Bug is a record of a bug. See API documentation at:
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#get-bug
type Bug struct {
	// ActualTime is the total number of hours that this bug has taken so far. If you are not in the time-tracking group, this field will not be included in the return value.
	ActualTime int `json:"actual_time,omitempty"`
	// Alias is the unique aliases of this bug. An empty array will be returned if this bug has no aliases.
	Alias []string `json:"alias,omitempty"`
	// AssignedTo is the login name of the user to whom the bug is assigned.
	AssignedTo string `json:"assigned_to,omitempty"`
	// AssignedToDetail is an object containing detailed user information for the assigned_to. To see the keys included in the user detail object, see below.
	AssignedToDetail *User `json:"assigned_to_detail,omitempty"`
	// Blocks is the IDs of bugs that are "blocked" by this bug.
	Blocks []int `json:"blocks,omitempty"`
	// CC is the login names of users on the CC list of this bug.
	CC []string `json:"cc,omitempty"`
	// CCDetail is array of objects containing detailed user information for each of the cc list members. To see the keys included in the user detail object, see below.
	CCDetail []User `json:"cc_detail,omitempty"`
	// Classification is the name of the current classification the bug is in.
	Classification string `json:"classification,omitempty"`
	// Component is an array of names of the current components of this bug.
	Component []string `json:"component,omitempty"`
	// CreationTime is when the bug was created.
	CreationTime string `json:"creation_time,omitempty"`
	// Creator is the login name of the person who filed this bug (the reporter).
	Creator string `json:"creator,omitempty"`
	// CreatorDetail is an object containing detailed user information for the creator. To see the keys included in the user detail object, see below.
	CreatorDetail *User `json:"creator_detail,omitempty"`
	// Deadline is the day that this bug is due to be completed, in the format YYYY-MM-DD.
	Deadline string `json:"deadline,omitempty"`
	// DependsOn is the IDs of bugs that this bug "depends on".
	DependsOn []int `json:"depends_on,omitempty"`
	// DupeOf is the bug ID of the bug that this bug is a duplicate of. If this bug isn't a duplicate of any bug, this will be null.
	DupeOf int `json:"dupe_of,omitempty"`
	// EstimatedTime is the number of hours that it was estimated that this bug would take. If you are not in the time-tracking group, this field will not be included in the return value.
	EstimatedTime int `json:"estimated_time,omitempty"`
	// Flags is an array of objects containing the information about flags currently set for the bug. Each flag objects contains the following items
	Flags []Flag `json:"flags,omitempty"`
	// Groups is the names of all the groups that this bug is in.
	Groups []string `json:"groups,omitempty"`
	// ID is the unique numeric ID of this bug.
	ID int `json:"id,omitempty"`
	// IsCCAccessible is if true, this bug can be accessed by members of the CC list, even if they are not in the groups the bug is restricted to.
	IsCCAccessible bool `json:"is_cc_accessible,omitempty"`
	// IsConfirmed is true if the bug has been confirmed. Usually this means that the bug has at some point been moved out of the UNCONFIRMED status and into another open status.
	IsConfirmed bool `json:"is_confirmed,omitempty"`
	// IsOpen is true if this bug is open, false if it is closed.
	IsOpen bool `json:"is_open,omitempty"`
	// IsCreatorAccessible is if true, this bug can be accessed by the creator of the bug, even if they are not a member of the groups the bug is restricted to.
	IsCreatorAccessible bool `json:"is_creator_accessible,omitempty"`
	// Keywords is each keyword that is on this bug.
	Keywords []string `json:"keywords,omitempty"`
	// LastChangeTime is when the bug was last changed.
	LastChangeTime string `json:"last_change_time,omitempty"`
	// OperatingSystem is the name of the operating system that the bug was filed against.
	OperatingSystem string `json:"op_sys,omitempty"`
	// Platform is the name of the platform (hardware) that the bug was filed against.
	Platform string `json:"platform,omitempty"`
	// Priority is the priority of the bug.
	Priority string `json:"priority,omitempty"`
	// Product is the name of the product this bug is in.
	Product string `json:"product,omitempty"`
	// QAContact is the login name of the current QA Contact on the bug.
	QAContact string `json:"qa_contact,omitempty"`
	// QAContactDetail is an object containing detailed user information for the qa_contact. To see the keys included in the user detail object, see below.
	QAContactDetail *User `json:"qa_contact_detail,omitempty"`
	// RemainingTime is the number of hours of work remaining until work on this bug is complete. If you are not in the time-tracking group, this field will not be included in the return value.
	RemainingTime int `json:"remaining_time,omitempty"`
	// Resolution is the current resolution of the bug, or an empty string if the bug is open.
	Resolution string `json:"resolution,omitempty"`
	// SeeAlso is the URLs in the See Also field on the bug.
	SeeAlso []string `json:"see_also,omitempty"`
	// Severity is the current severity of the bug.
	Severity string `json:"severity,omitempty"`
	// Status is the current status of the bug.
	Status string `json:"status,omitempty"`
	// Summary is the summary of this bug.
	Summary string `json:"summary,omitempty"`
	// TargetMilestone is the milestone that this bug is supposed to be fixed by, or for closed bugs, the milestone that it was fixed for.
	TargetMilestone string `json:"target_milestone,omitempty"`
	// TargetRelease are the releases that the bug will be fixed in.
	TargetRelease []string `json:"target_release,omitempty"`
	// UpdateToken is the token that you would have to pass to the process_bug.cgi page in order to update this bug. This changes every time the bug is updated. This field is not returned to logged-out users.
	UpdateToken string `json:"update_token,omitempty"`
	// URL is a URL that demonstrates the problem described in the bug, or is somehow related to the bug report.
	URL string `json:"url,omitempty"`
	// Version are the versions the bug was reported against.
	Version []string `json:"version,omitempty"`
	// Whiteboard is he value of the "status whiteboard" field on the bug.
	Whiteboard string `json:"whiteboard,omitempty"`
}

// User holds information about a user
type User struct {
	// The user ID for this user.
	ID int `json:"id,omitempty"`
	// The 'real' name for this user, if any.
	RealName string `json:"real_name,omitempty"`
	// The user's Bugzilla login.
	Name string `json:"name,omitempty"`
	// The user's e-mail.
	Email string `json:"email,omitempty"`
}

// Flag holds information about a flag set on a bug
type Flag struct {
	// The ID of the flag.
	ID int `json:"id,omitempty"`
	// The name of the flag.
	Name string `json:"name,omitempty"`
	// The type ID of the flag.
	TypeID int `json:"type_id,omitempty"`
	// The timestamp when this flag was originally created.
	CreationDate string `json:"creation_date,omitempty"`
	// The timestamp when the flag was last modified.
	ModificationDate string `json:"modification_date,omitempty"`
	// The current status of the flag.
	Status string `json:"status,omitempty"`
	// The login name of the user who created or last modified the flag.
	Setter string `json:"setter,omitempty"`
	// The login name of the user this flag has been requested to be granted or denied. Note, this field is only returned if a requestee is set.
	Requestee string `json:"requestee,omitempty"`
}

// BugUpdate contains fields to update on a Bug. See API documentation at:
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#update-bug
type BugUpdate struct {
	// Status is the current status of the bug.
	Status     string `json:"status,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

// ExternalBug contains details about an external bug linked to a Bugzilla bug.
// See API documentation at:
// https://bugzilla.redhat.com/docs/en/html/integrating/api/Bugzilla/Extension/ExternalBugs/WebService.html
type ExternalBug struct {
	// Type holds more metadata for the external bug tracker
	Type ExternalBugType `json:"type"`
	// BugzillaBugID is the ID of the Bugzilla bug this external bug is linked to
	BugzillaBugID int `json:"bug_id"`
	// ExternalBugID is a unique identifier for the bug under the tracker
	ExternalBugID string `json:"ext_bz_bug_id"`
	// The following fields are parsed from the external bug identifier
	Org, Repo string
	Num       int
}

// ExternalBugType holds identifying metadata for a tracker
type ExternalBugType struct {
	// URL is the identifying URL for this tracker
	URL string `json:"url"`
}

// AddExternalBugParameters are the parameters required to add an external
// tracker bug to a Bugtzilla bug
type AddExternalBugParameters struct {
	// APIKey is the API key to use when authenticating with Bugzilla
	APIKey string `json:"api_key"`
	// BugIDs are the IDs of Bugzilla bugs to update
	BugIDs []int `json:"bug_ids"`
	// ExternalBugs are the external bugs to add
	ExternalBugs []NewExternalBugIdentifier `json:"external_bugs"`
}

// NewExternalBugIdentifier holds fields used to identify new external bugs when
// adding them using the JSONRPC API
type NewExternalBugIdentifier struct {
	// Type is the URL prefix that identifies the external bug tracker type.
	// For GitHub, this is commonly https://github.com/
	Type string `json:"ext_type_url"`
	// ID is the identifier of the external bug within the bug tracker type.
	// For GitHub issues and pull requests, this ID is commonly the path
	// like `org/repo/pull/number` or `org/repo/issue/number`.
	ID string `json:"ext_bz_bug_id"`
}
