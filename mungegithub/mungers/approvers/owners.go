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
	"encoding/json"
	"math/rand"
	"sort"
	"strings"

	"bytes"
	"fmt"
	"path/filepath"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"
)

const (
	ownersFileName           = "OWNERS"
	ApprovalNotificationName = "ApprovalNotifier"
)

type RepoInterface interface {
	Approvers(path string) sets.String
	LeafApprovers(path string) sets.String
	FindApproverOwnersForPath(path string) string
}

type RepoAlias struct {
	repo  RepoInterface
	alias features.Aliases
}

func NewRepoAlias(repo RepoInterface, alias features.Aliases) *RepoAlias {
	return &RepoAlias{
		repo:  repo,
		alias: alias,
	}
}

func (r *RepoAlias) Approvers(path string) sets.String {
	return r.alias.Expand(r.repo.Approvers(path))
}

func (r *RepoAlias) LeafApprovers(path string) sets.String {
	return r.alias.Expand(r.repo.LeafApprovers(path))
}
func (r *RepoAlias) FindApproverOwnersForPath(path string) string {
	return r.repo.FindApproverOwnersForPath(path)
}

type Owners struct {
	filenames []string
	repo      RepoInterface
	seed      int64
}

func NewOwners(filenames []string, r RepoInterface, s int64) Owners {
	return Owners{filenames: filenames, repo: r, seed: s}
}

// GetApprovers returns a map from ownersFiles -> people that are approvers in them
func (o Owners) GetApprovers() map[string]sets.String {
	ownersToApprovers := map[string]sets.String{}

	for fn := range o.GetOwnersSet() {
		ownersToApprovers[fn] = o.repo.Approvers(fn)
	}

	return ownersToApprovers
}

// GetLeafApprovers returns a map from ownersFiles -> people that are approvers in them (only the leaf)
func (o Owners) GetLeafApprovers() map[string]sets.String {
	ownersToApprovers := map[string]sets.String{}

	for fn := range o.GetOwnersSet() {
		ownersToApprovers[fn] = o.repo.LeafApprovers(fn)
	}

	return ownersToApprovers
}

// GetAllPotentialApprovers returns the people from relevant owners files needed to get the PR approved
func (o Owners) GetAllPotentialApprovers() []string {
	approversOnly := []string{}
	for _, approverList := range o.GetLeafApprovers() {
		for approver := range approverList {
			approversOnly = append(approversOnly, approver)
		}
	}
	sort.Strings(approversOnly)
	return approversOnly
}

// GetReverseMap returns a map from people -> OWNERS files for which they are an approver
func (o Owners) GetReverseMap() map[string]sets.String {
	approverOwnersfiles := map[string]sets.String{}
	for ownersFile, approvers := range o.GetLeafApprovers() {
		for approver := range approvers {
			if _, ok := approverOwnersfiles[approver]; ok {
				approverOwnersfiles[approver].Insert(ownersFile)
			} else {
				approverOwnersfiles[approver] = sets.NewString(ownersFile)
			}
		}
	}
	return approverOwnersfiles
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

// GetSuggestedApprovers solves the exact cover problem, finding an approver capable of
// approving every OWNERS file in the PR
func (o Owners) GetSuggestedApprovers() sets.String {
	randomizedApprovers := o.GetShuffledApprovers()
	reverseMap := o.GetReverseMap()

	ap := NewApprovers(o)
	for !ap.IsApproved() {
		ap.AddApprover(findMostCoveringApprover(randomizedApprovers, reverseMap, ap.UnapprovedFiles()), "")
	}

	return ap.GetCurrentApproversSet()
}

// GetOwnersSet returns a set containing all the Owners files necessary to get the PR approved
func (o Owners) GetOwnersSet() sets.String {
	owners := sets.NewString()
	for _, fn := range o.filenames {
		owners.Insert(o.repo.FindApproverOwnersForPath(fn))
	}
	return removeSubdirs(owners.List())
}

// Shuffles the potential approvers so that we don't always suggest the same people
func (o Owners) GetShuffledApprovers() []string {
	approversList := o.GetAllPotentialApprovers()
	order := rand.New(rand.NewSource(o.seed)).Perm(len(approversList))
	people := make([]string, 0, len(approversList))
	for _, i := range order {
		people = append(people, approversList[i])
	}
	return people
}

// removeSubdirs takes a list of directories as an input and returns a set of directories with all
// subdirectories removed.  E.g. [/a,/a/b/c,/d/e,/d/e/f] -> [/a, /d/e]
func removeSubdirs(dirList []string) sets.String {
	toDel := sets.String{}
	for i := 0; i < len(dirList)-1; i++ {
		for j := i + 1; j < len(dirList); j++ {
			// ex /a/b has prefix /a so if remove /a/b since its already covered
			if strings.HasPrefix(dirList[i], dirList[j]) {
				toDel.Insert(dirList[i])
			} else if strings.HasPrefix(dirList[j], dirList[i]) {
				toDel.Insert(dirList[j])
			}
		}
	}
	finalSet := sets.NewString(dirList...)
	finalSet.Delete(toDel.List()...)
	return finalSet
}

// Approval has the information about each approval on a PR
type Approval struct {
	Login     string // Login of the approver
	How       string // How did the approver approved
	Reference string // Where did the approver approved
}

// String creates a link for the approval. Use `Login` if you just want the name.
func (a Approval) String() string {
	return fmt.Sprintf(
		`*<a href="%s" title="%s">%s</a>*`,
		a.Reference,
		a.How,
		a.Login,
	)
}

type Approvers struct {
	owners    Owners
	approvers map[string]Approval
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
		owners:    owners,
		approvers: map[string]Approval{},
	}
}

// AddLGTMer adds a new LGTM Approver
func (ap *Approvers) AddLGTMer(login, reference string) {
	ap.approvers[login] = Approval{
		Login:     login,
		How:       "LGTM",
		Reference: reference,
	}
}

// AddApprover adds a new Approver
func (ap *Approvers) AddApprover(login, reference string) {
	ap.approvers[login] = Approval{
		Login:     login,
		How:       "Approved",
		Reference: reference,
	}
}

// AddSAuthorSelfApprover adds the author self approval
func (ap *Approvers) AddAuthorSelfApprover(login, reference string) {
	ap.approvers[login] = Approval{
		Login:     login,
		How:       "Author self-approved",
		Reference: reference,
	}
}

// RemoveApprover removes an approver from the list.
func (ap *Approvers) RemoveApprover(login string) {
	delete(ap.approvers, login)
}

// GetCurrentApproversSet returns the set of approvers (login only)
func (ap Approvers) GetCurrentApproversSet() sets.String {
	currentApprovers := sets.NewString()

	for approver := range ap.approvers {
		currentApprovers.Insert(approver)
	}

	return currentApprovers
}

// GetFilesApprovers returns a map from files -> list of current approvers.
func (ap Approvers) GetFilesApprovers() map[string]sets.String {
	filesApprovers := map[string]sets.String{}
	currentApprovers := ap.GetCurrentApproversSet()

	for fn, potentialApprovers := range ap.owners.GetApprovers() {
		// The order of parameter matters here:
		// - currentApprovers is the list of github handle that have approved
		// - potentialApprovers is the list of handle in OWNERSa
		// files that can approve each file.
		//
		// We want to keep the syntax of the github handle
		// rather than the potential mis-cased username found in
		// the OWNERS file, that's why it's the first parameter.
		filesApprovers[fn] = IntersectSetsCase(currentApprovers, potentialApprovers)
	}

	return filesApprovers
}

// UnapprovedFiles returns owners files that still need approval
func (ap Approvers) UnapprovedFiles() sets.String {
	unapproved := sets.NewString()
	for fn, approvers := range ap.GetFilesApprovers() {
		if len(approvers) == 0 {
			unapproved.Insert(fn)
		}
	}
	return unapproved
}

// UnapprovedFiles returns owners files that still need approval
func (ap Approvers) GetFiles() []File {
	allOwnersFiles := []File{}
	filesApprovers := ap.GetFilesApprovers()
	for _, fn := range ap.owners.GetOwnersSet().List() {
		if len(filesApprovers[fn]) == 0 {
			allOwnersFiles = append(allOwnersFiles, UnapprovedFile{fn})
		} else {
			allOwnersFiles = append(allOwnersFiles, ApprovedFile{fn, filesApprovers[fn]})
		}
	}

	return allOwnersFiles
}

func (ap Approvers) GetCCs() []string {
	approvers := []string{}

	reverseMap := ap.owners.GetReverseMap()
	unapproved := ap.UnapprovedFiles()

	for _, suggestedApprover := range ap.owners.GetSuggestedApprovers().List() {
		if reverseMap[suggestedApprover].Intersection(unapproved).Len() != 0 {
			approvers = append(approvers, suggestedApprover)
		}
	}

	return approvers
}

// IsApproved returns a bool indicating whether or not the PR is approved
func (ap Approvers) IsApproved() bool {
	return ap.UnapprovedFiles().Len() == 0
}

// ListString returns the list of all approvers along with links and tooltips.
func (ap Approvers) ListString() string {
	approvals := []string{}

	for _, approver := range ap.GetCurrentApproversSet().List() {
		approvals = append(approvals, ap.approvers[approver].String())
	}

	return strings.Join(approvals, ", ")
}

type File interface {
	toString(string, string) string
}

type ApprovedFile struct {
	filepath  string
	approvers sets.String
}

type UnapprovedFile struct {
	filepath string
}

func (a ApprovedFile) toString(org, project string) string {
	fullOwnersPath := filepath.Join(a.filepath, ownersFileName)
	link := fmt.Sprintf("https://github.com/%s/%s/blob/master/%v", org, project, fullOwnersPath)
	return fmt.Sprintf("- ~~[%s](%s)~~ [%v]\n", fullOwnersPath, link, strings.Join(a.approvers.List(), ","))
}

func (ua UnapprovedFile) toString(org, project string) string {
	fullOwnersPath := filepath.Join(ua.filepath, ownersFileName)
	link := fmt.Sprintf("https://github.com/%s/%s/blob/master/%v", org, project, fullOwnersPath)
	return fmt.Sprintf("- **[%s](%s)** \n", fullOwnersPath, link)
}

// getMessage returns the comment body that we want the approval-handler to display on PRs
// The comment shows:
// 	- a list of approvers files (and links) needed to get the PR approved
// 	- a list of approvers files with strikethroughs that already have an approver's approval
// 	- a suggested list of people from each OWNERS files that can fully approve the PR
// 	- how an approver can indicate their approval
// 	- how an approver can cancel their approval
func GetMessage(ap Approvers, org, project string) string {
	formatStr := "The following people have approved this PR: %v\n\nNeeds approval from an approver in each of these OWNERS Files:\n"
	context := bytes.NewBufferString(fmt.Sprintf(formatStr, ap.ListString()))
	for _, ownersFile := range ap.GetFiles() {
		context.WriteString(ownersFile.toString(org, project))
	}

	CCs := ap.GetCCs()
	if len(CCs) != 0 {
		context.WriteString("\nWe suggest the following people:\ncc ")
		for _, person := range CCs {
			context.WriteString("" + person + " ")
		}
	}
	context.WriteString("\n You can indicate your approval by writing `/approve` in a comment\n You can cancel your approval by writing `/approve cancel` in a comment")
	title := "This PR is **NOT APPROVED**"
	if ap.IsApproved() {
		title = "This PR is **APPROVED**"
	}
	context.WriteString(getGubernatorMeta(CCs))
	return (&c.Notification{ApprovalNotificationName, title, context.String()}).String()
}

// gets the meta data gubernator uses for
func getGubernatorMeta(toBeAssigned []string) string {
	forMachine := map[string][]string{"approvers": toBeAssigned}
	bytes, err := json.Marshal(forMachine)
	if err == nil {
		return fmt.Sprintf("\n<!-- META=%s -->", bytes)
	}
	return ""
}
