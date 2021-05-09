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

package approvers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pkg/layeredsets"
)

const (
	ownersFileName = "OWNERS"
	// ApprovalNotificationName defines the name used in the title for the approval notifications.
	ApprovalNotificationName = "ApprovalNotifier"
)

// Repo allows querying and interacting with OWNERS information in a repo.
type Repo interface {
	Approvers(path string) layeredsets.String
	LeafApprovers(path string) sets.String
	FindApproverOwnersForFile(file string) string
	IsNoParentOwners(path string) bool
}

// Owners provides functionality related to owners of a specific code change.
type Owners struct {
	filenames []string
	repo      Repo
	seed      int64

	log *logrus.Entry
}

// NewOwners consturcts a new Owners instance. filenames is the slice of files changed.
func NewOwners(log *logrus.Entry, filenames []string, r Repo, s int64) Owners {
	return Owners{filenames: filenames, repo: r, seed: s, log: log}
}

// GetApprovers returns a map from filenames -> people that can approve it
func (o Owners) GetApprovers() map[string]sets.String {
	filesToApprovers := map[string]sets.String{}

	for _, fn := range o.filenames {
		filesToApprovers[fn] = o.repo.Approvers(fn).Set()
	}

	return filesToApprovers
}

// GetLeafApprovers returns a map from files -> people that are approvers in them (only the leaf)
func (o Owners) GetLeafApprovers() map[string]sets.String {
	ownersToApprovers := map[string]sets.String{}

	for _, fn := range o.filenames {
		ownersToApprovers[fn] = o.repo.LeafApprovers(fn)
	}

	return ownersToApprovers
}

// GetAllPotentialApprovers returns the people from relevant owners files needed to get the PR approved
// It returns a list of people based on leaf owners files after subdirectories are ignored.
// Example: If the PR has the following file changes:
// 		- a/b/c.go
//		- a/b/c/d.go
//		- p/q.go
// this function will return the list of approvers in a/b/c/OWNERS and p/OWNERS
func (o Owners) GetAllPotentialApprovers() []string {
	approvers := sets.NewString()
	owners := o.GetOwnersSet()
	for _, fn := range o.filenames {
		if owners.Has(o.repo.FindApproverOwnersForFile(fn)) {
			approvers.Insert(o.repo.LeafApprovers(fn).List()...)
		}
	}
	approversOnly := approvers.List()
	sort.Strings(approversOnly)
	if len(approversOnly) == 0 {
		o.log.Debug("No potential approvers exist. Does the repo have OWNERS files?")
	}
	return approversOnly
}

// GetReverseMap returns a map from people -> files. files for which they are an approver
func (o Owners) GetReverseMap(approvers map[string]sets.String) map[string]sets.String {
	approverFiles := map[string]sets.String{}
	for file, approvers := range approvers {
		for approver := range approvers {
			if _, ok := approverFiles[approver]; ok {
				approverFiles[approver].Insert(file)
			} else {
				approverFiles[approver] = sets.NewString(file)
			}
		}
	}
	return approverFiles
}

func findMostCoveringApprover(allApprovers []string, reverseMap map[string]sets.String, unapproved sets.String) string {
	maxCovered := 0
	var bestPerson string
	for _, approver := range allApprovers {
		filesCanApprove := reverseMap[approver]
		if filesCanApprove.Intersection(unapproved).Len() > maxCovered {
			maxCovered = len(filesCanApprove)
			bestPerson = approver
		}
	}
	return bestPerson
}

// temporaryUnapprovedFiles returns the list of files that wouldn't be
// approved by the given set of approvers.
func (o Owners) temporaryUnapprovedFiles(approvals []Approval) sets.String {
	ap := NewApprovers(o)
	for _, aprvl := range approvals {
		for _, info := range aprvl.Infos {
			ap.AddApprover(aprvl.Login, info.Reference, info.Path)
		}
	}
	return ap.UnapprovedFiles()
}

// KeepCoveringApprovers finds who we should keep as suggested approvers given a pre-selection
// knownApprovers must be a subset of potentialApprovers.
func (o Owners) KeepCoveringApprovers(reverseMap map[string]sets.String, knownApprovals []Approval, potentialApprovers []string) sets.String {
	if len(potentialApprovers) == 0 {
		o.log.Debug("No potential approvers exist to filter for relevance. Does this repo have OWNERS files?")
	}
	keptApprovers := sets.NewString()

	unapproved := o.temporaryUnapprovedFiles(knownApprovals)

	for _, suggestedApprover := range o.GetSuggestedApprovers(reverseMap, potentialApprovers).List() {
		if reverseMap[suggestedApprover].Intersection(unapproved).Len() != 0 {
			keptApprovers.Insert(suggestedApprover)
		}
	}

	return keptApprovers
}

// GetSuggestedApprovers solves the exact cover problem, finding an approver capable of
// approving every OWNERS file in the PR
func (o Owners) GetSuggestedApprovers(reverseMap map[string]sets.String, potentialApprovers []string) sets.String {
	ap := NewApprovers(o)
	for !ap.RequirementsMet() {
		newApprover := findMostCoveringApprover(potentialApprovers, reverseMap, ap.UnapprovedFiles())
		if newApprover == "" {
			o.log.Warnf("Couldn't find/suggest approvers for each files. Unapproved: %q", ap.UnapprovedFiles().List())
			return ap.GetCurrentApproversSet()
		}
		ap.AddApprover(newApprover, "", "")
	}

	return ap.GetCurrentApproversSet()
}

// GetOwnersSet returns a set containing all the Owners files necessary to get the PR approved
func (o Owners) GetOwnersSet() sets.String {
	owners := sets.NewString()
	for _, fn := range o.filenames {
		owners.Insert(o.repo.FindApproverOwnersForFile(fn))
	}
	o.removeSubdirs(owners)
	return owners
}

// GetFolderFiles returns a map from folder(owners file folder) -> files in that folder
// for this PR
func (o Owners) GetFolderFiles() map[string]sets.String {
	folders := o.GetOwnersSet()
	folderfiles := map[string]sets.String{}
	for folder := range folders {
		folderfiles[folder] = sets.NewString()
	}

	for _, file := range o.filenames {
		for folder := range folders {
			if strings.HasPrefix(file, folder) {
				folderfiles[folder].Insert(file)
			}
		}
	}

	return folderfiles
}

// GetShuffledApprovers shuffles the potential approvers so that we don't
// always suggest the same people.
func (o Owners) GetShuffledApprovers() []string {
	approversList := o.GetAllPotentialApprovers()
	order := rand.New(rand.NewSource(o.seed)).Perm(len(approversList))
	people := make([]string, 0, len(approversList))
	for _, i := range order {
		people = append(people, approversList[i])
	}
	return people
}

// GetShuffledApproversSubset shuffles the potential approvers, without the people
// in remove set, so that we don't always suggest the same people.
func (o Owners) GetShuffledApproversSubset(remove sets.String) []string {
	approversList := o.GetAllPotentialApprovers()
	order := rand.New(rand.NewSource(o.seed)).Perm(len(approversList))
	people := []string{}
	for _, i := range order {
		if !remove.Has(approversList[i]) {
			people = append(people, approversList[i])
		}
	}
	return people
}

// removeSubdirs takes a set of directories as an input and removes all subdirectories.
// E.g. [a, a/b/c, d/e, d/e/f] -> [a, d/e]
// Subdirs will not be removed if they are configured to have no parent OWNERS files or if any
// OWNERS file in the relative path between the subdir and the higher level dir is configured to
// have no parent OWNERS files.
func (o Owners) removeSubdirs(dirs sets.String) {
	canonicalize := func(p string) string {
		if p == "." {
			return ""
		}
		return p
	}
	for _, dir := range dirs.List() {
		path := dir
		for {
			if o.repo.IsNoParentOwners(path) || canonicalize(path) == "" {
				break
			}
			path = filepath.Dir(path)
			if dirs.Has(canonicalize(path)) {
				dirs.Delete(dir)
				break
			}
		}
	}
}

type ApprovalInfo struct {
	Reference string
	Path      string
}

// Approval has the information about each approval on a PR
type Approval struct {
	Login string         // Login of the approver (can include uppercase)
	How   string         // How did the approver approved
	Infos []ApprovalInfo // More information about the approval
}

func (a Approval) CoversFile(file string) bool {
	for _, info := range a.Infos {
		if wildcardPathMatch(info.Path, file) {
			return true
		}
	}
	return false
}

// String creates a link for the approval. Use `Login` if you just want the name.
func (a Approval) String() string {
	return fmt.Sprintf(
		`*<a href="%s" title="%s">%s</a>*`,
		a.Infos[0].Reference,
		a.How,
		a.Login,
	)
}

// NoIssueApproval has the information about each "no-issue" approval on a PR
type NoIssueApproval struct {
	Login     string // Login of the approver (can include uppercase)
	How       string // How did the approver approve
	Reference string // Where did the approver approved
}

// String creates a link for the approval. Use `Login` if you just want the name.
func (a NoIssueApproval) String() string {
	return fmt.Sprintf(
		`*<a href="%s" title="%s">%s</a>*`,
		a.Reference,
		a.How,
		a.Login,
	)
}

// Approvers is struct that provide functionality with regard to approvals of a specific
// code change.
type Approvers struct {
	owners           Owners
	approvers        map[string]*Approval // The keys of this map are normalized to lowercase.
	noissueapprovers map[string]*NoIssueApproval
	assignees        sets.String
	AssociatedIssue  int
	RequireIssue     bool

	ManuallyApproved func() bool
}

// IntersectSetsCase runs the intersection between to sets.String in a
// case-insensitive way. It returns the name with the case of "one".
func IntersectSetsCase(one, other sets.String) sets.String {
	lower := sets.NewString()
	for item := range other {
		lower.Insert(strings.ToLower(item))
	}

	intersection := sets.NewString()
	for item := range one {
		if lower.Has(strings.ToLower(item)) {
			intersection.Insert(item)
		}
	}
	return intersection
}

// NewApprovers create a new "Approvers" with no approval.
func NewApprovers(owners Owners) Approvers {
	return Approvers{
		owners:           owners,
		approvers:        map[string]*Approval{},
		noissueapprovers: map[string]*NoIssueApproval{},
		assignees:        sets.NewString(),

		ManuallyApproved: func() bool {
			return false
		},
	}
}

// shouldNotOverrideApproval decides whether or not we should keep the
// original approval:
// If someone approves a PR multiple times, we only want to keep the
// latest approval, unless a previous approval was "no-issue", and the
// most recent isn't.
/*
func (ap *Approvers) shouldNotOverrideApproval(login string, noIssue bool) bool {
	login = strings.ToLower(login)
	approval, alreadyApproved := ap.approvers[login]

	return alreadyApproved && approval.NoIssue && !noIssue
}
*/

// AddLGTMer adds a new LGTM Approver
func (ap *Approvers) AddLGTMer(login, reference, path string) {
	ap.addApproval(login, "LGTM", reference, path)
}

// AddApprover adds a new Approver
func (ap *Approvers) AddApprover(login, reference, path string) {
	ap.addApproval(login, "Approved", reference, path)
}

func (ap *Approvers) addApproval(login, how, reference, approvedPath string) {
	if approvedPath == "" {
		approvedPath = "*"
	}
	if approval, ok := ap.approvers[strings.ToLower(login)]; !ok {
		ap.approvers[strings.ToLower(login)] = &Approval{
			Login: login,
			How:   how,
			Infos: []ApprovalInfo{
				{
					Reference: reference,
					Path:      approvedPath,
				},
			},
		}
	} else {
		approval.Infos = append(approval.Infos, ApprovalInfo{
			Reference: reference,
			Path:      approvedPath,
		})
	}
}

func (ap *Approvers) AddNoIssueApprover(login, reference string) {
	ap.addNoIssueApproval(login, "Approved", reference)
}

func (ap *Approvers) AddNoIssueLGTMer(login, reference string) {
	ap.addNoIssueApproval(login, "LGTM", reference)
}

func (ap *Approvers) addNoIssueApproval(login, how, reference string) {
	ap.noissueapprovers[strings.ToLower(login)] = &NoIssueApproval{
		Login:     login,
		How:       how,
		Reference: reference,
	}
}

// AddAuthorSelfApprover adds the author self approval
func (ap *Approvers) AddAuthorSelfApprover(login, reference, path string) {
	ap.addApproval(login, "Author self-approved", reference, path)
}

func (ap *Approvers) AddNoIssueAuthorSelfApprover(login, reference string) {
	ap.addNoIssueApproval(login, "Author self-approved", reference)
}

// RemoveApprover removes an approver from the list.
// RemoveApprover removes an approver from the list.
func (ap *Approvers) RemoveApprover(login string) {
	delete(ap.approvers, strings.ToLower(login))
}

// AddAssignees adds assignees to the list
func (ap *Approvers) AddAssignees(logins ...string) {
	for _, login := range logins {
		ap.assignees.Insert(strings.ToLower(login))
	}
}

// GetCurrentApproversSet returns the set of approvers (login only, normalized to lower case)
func (ap Approvers) GetCurrentApproversSet() sets.String {
	currentApprovers := sets.NewString()
	for approver := range ap.approvers {
		currentApprovers.Insert(approver)
	}
	return currentApprovers
}

// GetCurrentApproversSetCased returns the set of approvers logins with the original cases.
func (ap Approvers) GetCurrentApproversSetCased() sets.String {
	currentApprovers := sets.NewString()

	for _, approval := range ap.approvers {
		currentApprovers.Insert(approval.Login)
	}

	return currentApprovers
}

// GetNoIssueApproversSet returns the set of "no-issue" approvers (login
// only)
func (ap Approvers) GetNoIssueApproversSet() sets.String {
	approvers := sets.NewString()

	for approver := range ap.NoIssueApprovers() {
		approvers.Insert(approver)
	}

	return approvers
}

// GetFilesApprovers returns a map from files -> list of current approvers.
func (ap Approvers) GetFilesApprovers() map[string]sets.String {
	filesApprovers := map[string]sets.String{}
	for fn, potentialApprovers := range ap.owners.GetApprovers() {
		// potentialApprovers is the list of github handles who can approve file fn.
		filesApprovers[fn] = approversForFile(fn, potentialApprovers, ap.approvers)
	}

	return filesApprovers
}

// NoIssueApprovers returns the list of people who have "no-issue"
// approved the pull-request. They are included in the list iff they can
// approve one of the files.
func (ap Approvers) NoIssueApprovers() map[string]*NoIssueApproval {
	nia := map[string]*NoIssueApproval{}
	reverseMap := ap.owners.GetReverseMap(ap.owners.GetApprovers())

	for login, approval := range ap.noissueapprovers {

		if files, ok := reverseMap[login]; !ok || len(files) == 0 {
			continue
		}

		nia[login] = approval
	}

	return nia
}

// UnapprovedFiles returns files that still need approval
func (ap Approvers) UnapprovedFiles() sets.String {
	unapproved := sets.NewString()
	for fn, approvers := range ap.GetFilesApprovers() {
		if len(approvers) == 0 {
			unapproved.Insert(fn)
		}
	}
	return unapproved
}

// GetFolderStatus returns the approval status of the folders.
// A folder can be approve, partially approved or unapproved
func (ap Approvers) GetFolderStatus(baseURL *url.URL, branch string) []File {
	allFiles := []File{}
	folderFiles := ap.owners.GetFolderFiles()
	fileApprovers := ap.GetFilesApprovers()

	for folder := range folderFiles {
		allFiles = append(allFiles, folderStatus(folder, folderFiles, fileApprovers, baseURL, branch))
	}

	return allFiles
}

// GetFiles returns owners files that still need approval.
func (ap Approvers) GetFiles(baseURL *url.URL, branch string) []File {
	allFiles := []File{}
	for owner := range ap.UnapprovedOwners() {
		allFiles = append(allFiles, UnapprovedFile{
			baseURL:  baseURL,
			branch:   branch,
			filepath: owner,
		})
	}

	return allFiles
}

// UnapprovedOwners return the owners files for the unapproved files
func (ap Approvers) UnapprovedOwners() sets.String {
	owners := sets.NewString()
	for fn := range ap.UnapprovedFiles() {
		owners.Insert(ap.owners.repo.FindApproverOwnersForFile(fn))
	}
	ap.owners.removeSubdirs(owners)
	return owners
}

// GetCCs gets the list of suggested approvers for a pull-request.  It
// now considers current assignees as potential approvers. Here is how
// it works:
// - We find suggested approvers from all potential approvers, but
// remove those that are not useful considering current approvers and
// assignees. This only uses leaf approvers to find the closest
// approvers to the changes.
// - We find a subset of suggested approvers from current
// approvers, suggested approvers and assignees, but we remove those
// that are not useful considering suggested approvers and current
// approvers. This uses the full approvers list, and will result in root
// approvers to be suggested when they are assigned.
// We return the union of the two sets: suggested and suggested
// assignees.
// The goal of this second step is to only keep the assignees that are
// the most useful.
func (ap Approvers) GetCCs() []string {
	approversAndAssigneeApprovals := approvalsAndBlanketApprovals(ap.ListApprovals(), ap.assignees)
	approversAndAssignees := approversOfApprovals(approversAndAssigneeApprovals)
	randomizedApprovers := ap.owners.GetShuffledApproversSubset(approversAndAssignees)
	leafReverseMap := ap.owners.GetReverseMap(ap.owners.GetLeafApprovers())
	suggested := ap.owners.KeepCoveringApprovers(leafReverseMap, approversAndAssigneeApprovals, randomizedApprovers)

	approversAndSuggestedApprovals := approvalsAndBlanketApprovals(ap.ListApprovals(), suggested)
	fullReverseMap := ap.owners.GetReverseMap(ap.owners.GetApprovers())
	keepAssignees := ap.owners.KeepCoveringApprovers(fullReverseMap, approversAndSuggestedApprovals, ap.assignees.List())

	return suggested.Union(keepAssignees).List()
}

// AreFilesApproved returns a bool indicating whether or not files associated with
// the PR are approved.  A PR with no files is not considered approved. If this
// returns true, the PR may still not be fully approved depending on the associated issue
// requirement
func (ap Approvers) AreFilesApproved() bool {
	return len(ap.owners.filenames) != 0 && ap.UnapprovedFiles().Len() == 0
}

// RequirementsMet returns a bool indicating whether the PR has met all approval requirements:
// - all OWNERS files associated with the PR have been approved AND
// EITHER
// 	- the munger config is such that an issue is not required to be associated with the PR
// 	- that there is an associated issue with the PR
// 	- an OWNER has indicated that the PR is trivial enough that an issue need not be associated with the PR
func (ap Approvers) RequirementsMet() bool {
	return ap.AreFilesApproved() && (!ap.RequireIssue || ap.AssociatedIssue != 0 || len(ap.NoIssueApprovers()) != 0)
}

// IsApproved returns a bool indicating whether the PR is fully approved.
// If a human manually added the approved label, this returns true, ignoring normal approval rules.
func (ap Approvers) IsApproved() bool {
	reqsMet := ap.RequirementsMet()
	if !reqsMet && ap.ManuallyApproved() {
		return true
	}
	return reqsMet
}

// ListApprovals returns the list of approvals
func (ap Approvers) ListApprovals() []Approval {
	approvals := []Approval{}
	for _, approval := range ap.approvers {
		approvals = append(approvals, *approval)
	}
	return approvals
}

// ListNoIssueApprovals returns the list of "no-issue" approvals
func (ap Approvers) ListNoIssueApprovals() []NoIssueApproval {
	approvals := []NoIssueApproval{}

	for _, approver := range ap.GetNoIssueApproversSet().List() {
		approvals = append(approvals, *ap.noissueapprovers[approver])
	}

	return approvals
}

// File in an interface for files
type File interface {
	String() string
}

// ApprovedFolder contains the information of a an approved folder.
type ApprovedFolder struct {
	baseURL    *url.URL
	folderpath string
	// approvers is the set of users that approved this file change.
	approvers sets.String
	branch    string
}

// PartiallyApprovedFolder contains the information of a an approved folder.
type PartiallyApprovedFolder struct {
	baseURL    *url.URL
	folderpath string
	approvers  sets.String
	branch     string
}

// UnapprovedFolder contains the information of a an unapproved folder.
type UnapprovedFolder struct {
	baseURL    *url.URL
	folderpath string
	branch     string
}

func (a ApprovedFolder) String() string {
	link := fmt.Sprintf("%s/blob/%s/%v",
		a.baseURL.String(),
		a.branch,
		a.folderpath,
	)
	return fmt.Sprintf("- ~~[%s/](%s)~~ (approved) [%v]\n", a.folderpath, link, strings.Join(a.approvers.List(), ","))
}

func (pa PartiallyApprovedFolder) String() string {
	link := fmt.Sprintf("%s/blob/%s/%v",
		pa.baseURL.String(),
		pa.branch,
		pa.folderpath,
	)
	return fmt.Sprintf("- **[%s/](%s)** (partially approved, need additional approvals) [%v]\n", pa.folderpath, link, strings.Join(pa.approvers.List(), ","))
}

func (ua UnapprovedFolder) String() string {
	link := fmt.Sprintf("%s/blob/%s/%v",
		ua.baseURL.String(),
		ua.branch,
		ua.folderpath,
	)
	return fmt.Sprintf("- **[%s/](%s)**\n", ua.folderpath, link)
}

// UnapprovedFile contains information approved an unapproved owners file
type UnapprovedFile struct {
	baseURL  *url.URL
	filepath string
	branch   string
}

func (uaf UnapprovedFile) String() string {
	fullOwnersPath := filepath.Join(uaf.filepath, ownersFileName)
	if strings.HasSuffix(uaf.filepath, ".md") {
		fullOwnersPath = uaf.filepath
	}
	link := fmt.Sprintf("%s/blob/%s/%v",
		uaf.baseURL.String(),
		uaf.branch,
		fullOwnersPath,
	)
	return fmt.Sprintf("- **[%s](%s)**\n", fullOwnersPath, link)
}

// GenerateTemplate takes a template, name and data, and generates
// the corresponding string.
func GenerateTemplate(templ, name string, data interface{}) (string, error) {
	funcMaps := template.FuncMap{
		"sub": func(a, b int) int {
			return a - b
		},
		"sortApprovals": func(l []Approval) []Approval {
			sort.Slice(l, func(i, j int) bool {
				return l[i].String() < l[j].String()
			})
			return l
		},
		"sortFiles": func(l []File) []File {
			sort.Slice(l, func(i, j int) bool {
				return l[i].String() < l[j].String()
			})
			return l
		},
	}
	buf := bytes.NewBufferString("")
	if messageTempl, err := template.New(name).Funcs(funcMaps).Parse(templ); err != nil {
		return "", fmt.Errorf("failed to parse template for %s: %v", name, err)
	} else if err := messageTempl.Execute(buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template for %s: %v", name, err)
	}
	return buf.String(), nil
}

// GetMessage returns the comment body that we want the approve plugin to display on PRs
// The comment shows:
// 	- a list of approvers files (and links) needed to get the PR approved
// 	- a list of approvers files with strikethroughs that already have an approver's approval
// 	- a suggested list of people from each OWNERS files that can fully approve the PR
// 	- how an approver can indicate their approval
// 	- how an approver can cancel their approval
func GetMessage(ap Approvers, linkURL *url.URL, commandHelpLink, prProcessLink, org, repo, branch string) *string {
	linkURL.Path = org + "/" + repo
	message, err := GenerateTemplate(`{{if (and (not .ap.RequirementsMet) (call .ap.ManuallyApproved )) }}
**Approval requirements bypassed by manually added approval.**

{{end -}}
This pull-request has been approved by:{{range $index, $approval := sortApprovals .ap.ListApprovals}}{{if $index}}, {{else}} {{end}}{{$approval}}{{end}}

{{- if (and (not .ap.AreFilesApproved) (not (call .ap.ManuallyApproved))) }}
To complete the [pull request process]({{ .prProcessLink }}), please assign {{range $index, $cc := .ap.GetCCs}}{{if $index}}, {{end}}**{{$cc}}**{{end}}
You can assign the PR to them by writing `+"`/assign {{range $index, $cc := .ap.GetCCs}}{{if $index}} {{end}}@{{$cc}}{{end}}`"+` in a comment when ready.
{{- end}}

{{if not .ap.RequireIssue -}}
{{else if .ap.AssociatedIssue -}}
Associated issue: *#{{.ap.AssociatedIssue}}*

{{ else if len .ap.NoIssueApprovers -}}
Associated issue requirement bypassed by:{{range $index, $approval := .ap.ListNoIssueApprovals}}{{if $index}}, {{else}} {{end}}{{$approval}}{{end}}

{{ else if call .ap.ManuallyApproved -}}
*No associated issue*. Requirement bypassed by manually added approval.

{{ else -}}
*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with `+"`/approve2 no-issue`"+`

{{ end -}}

The full list of commands accepted by this bot can be found [here]({{ .commandHelpLink }}?repo={{ .org }}%2F{{ .repo }}).

{{ if (or .ap.AreFilesApproved (call .ap.ManuallyApproved)) -}}
The pull request process is described [here]({{ .prProcessLink }})

{{ end -}}

Out of **{{len .ap.GetFilesApprovers}}** files: **{{sub (len .ap.GetFilesApprovers) (len .ap.UnapprovedFiles)}}** are approved and **{{len .ap.UnapprovedFiles}}** are unapproved.  

{{if ne (len .ap.UnapprovedFiles) 0 -}}
Needs approval from approvers in these files:
{{range (sortFiles (.ap.GetFiles .baseURL .branch))}}{{.}}{{end}}

Approvers can indicate their approval by writing `+"`/approve2`"+` in a comment
Approvers can also choose to approve only specific files by writing `+"`/approve2 files <path-to-file>`"+` in a comment
Approvers can cancel approval by writing `+"`/approve2 cancel`"+` in a comment
{{end -}}  

The status of the PR is:  

{{range (sortFiles (.ap.GetFolderStatus .baseURL .branch))}}{{.}}{{end}}
`, "message", map[string]interface{}{"ap": ap, "baseURL": linkURL, "commandHelpLink": commandHelpLink, "prProcessLink": prProcessLink, "org": org, "repo": repo, "branch": branch})
	if err != nil {
		ap.owners.log.WithError(err).Errorf("Error generating message.")
		return nil
	}
	message += getGubernatorMetadata(ap.GetCCs())

	title, err := GenerateTemplate("This PR is **{{if not .IsApproved}}NOT {{end}}APPROVED**", "title", ap)
	if err != nil {
		ap.owners.log.WithError(err).Errorf("Error generating title.")
		return nil
	}

	return notification(ApprovalNotificationName, title, message)
}

func notification(name, arguments, context string) *string {
	str := "[" + strings.ToUpper(name) + "]"

	args := strings.TrimSpace(arguments)
	if args != "" {
		str += " " + args
	}

	ctx := strings.TrimSpace(context)
	if ctx != "" {
		str += "\n\n" + ctx
	}

	return &str
}

// getGubernatorMetadata returns a JSON string with machine-readable information about approvers.
// This MUST be kept in sync with gubernator/github/classifier.py, particularly get_approvers.
func getGubernatorMetadata(toBeAssigned []string) string {
	bytes, err := json.Marshal(map[string][]string{"approvers": toBeAssigned})
	if err == nil {
		return fmt.Sprintf("\n<!-- META=%s -->", bytes)
	}
	return ""
}

// wildcardPathMatch return true if the targetPath matches the given pattern
// A '*' as a directory will recursively match all sub directories
//    Example: file a/b1/b2/c.go matches the pattern a/*/c.go
// A '*' is file name matches 0 or more characters
//    Example: file a/file_test.go matches the pattern a/*_test.go
func wildcardPathMatch(pattern, targetPath string) bool {
	if (len(pattern) == 0) != (len(targetPath) == 0) {
		return false
	}

	if len(pattern) == 0 && len(targetPath) == 0 {
		return true
	}

	var patternMatch, patternRemain string
	var targetPathMatch, targetPathRemain string

	patternSplitIndex := strings.IndexRune(pattern, '/')
	if patternSplitIndex != -1 {
		patternMatch, patternRemain = pattern[:patternSplitIndex], pattern[patternSplitIndex+1:]
	} else {
		patternMatch = pattern
	}

	targetPathSplitIndex := strings.IndexRune(targetPath, '/')
	if targetPathSplitIndex != -1 {
		targetPathMatch, targetPathRemain = targetPath[:targetPathSplitIndex], targetPath[targetPathSplitIndex+1:]
	} else {
		targetPathMatch = targetPath
	}

	if patternMatch == "*" && len(patternRemain) == 0 {
		// everything below is a match. return true
		return true
	}

	if patternMatch == "*" {
		return wildcardPathMatch(patternRemain, targetPathRemain) || wildcardPathMatch(pattern, targetPathRemain)
	}

	matched, err := path.Match(patternMatch, targetPathMatch)
	if err != nil {
		return false
	}
	if matched {
		return wildcardPathMatch(patternRemain, targetPathRemain)
	}
	return false
}

func wildcardPathSubset(pattern, targetPath string) bool {
	targetPath = strings.ReplaceAll(targetPath, "*", "xyz")
	return wildcardPathMatch(pattern, targetPath)
}

// approversForFile return the set of approvers in potentialApprovers who approved the file
func approversForFile(file string, potentialApprovers sets.String, currentApprovals map[string]*Approval) sets.String {
	approvers := sets.NewString()

	potentialApproversLowerCase := sets.NewString()
	for item := range potentialApprovers {
		potentialApproversLowerCase.Insert(strings.ToLower(item))
	}

	for login, approval := range currentApprovals {
		if potentialApproversLowerCase.Has(strings.ToLower(login)) && approval.CoversFile(file) {
			approvers.Insert(login)
		}
	}

	return approvers
}

func approvalsAndBlanketApprovals(approvals []Approval, blanketApprovers sets.String) []Approval {
	allApprovals := []Approval{}
	apprvs := sets.NewString()
	for _, approval := range approvals {
		apprvs.Insert(approval.Login)
		allApprovals = append(allApprovals, approval)
	}
	for apprvr := range blanketApprovers {
		if !apprvs.Has(apprvr) {
			allApprovals = append(allApprovals, Approval{
				Login: apprvr,
				Infos: []ApprovalInfo{{Path: "*"}},
			})
		}
	}
	return allApprovals
}

func approversOfApprovals(approvals []Approval) sets.String {
	approvers := sets.NewString()
	for _, approval := range approvals {
		approvers.Insert(strings.ToLower(approval.Login))
	}
	return approvers
}

func folderStatus(folder string, folderFiles map[string]sets.String, fileApprovers map[string]sets.String, baseURL *url.URL, branch string) File {
	approvedFiles, unapprovedFiles := 0, 0
	approvers := sets.NewString()
	for file := range folderFiles[folder] {
		if len(fileApprovers[file]) != 0 {
			approvedFiles++
			approvers = approvers.Union(fileApprovers[file])
		} else {
			unapprovedFiles++
		}
	}

	if unapprovedFiles == 0 {
		return ApprovedFolder{
			baseURL:    baseURL,
			folderpath: folder,
			approvers:  approvers,
			branch:     branch,
		}
	}

	if approvedFiles == 0 {
		return UnapprovedFolder{
			baseURL:    baseURL,
			branch:     branch,
			folderpath: folder,
		}
	}

	return PartiallyApprovedFolder{
		baseURL:    baseURL,
		folderpath: folder,
		approvers:  approvers,
		branch:     branch,
	}
}
