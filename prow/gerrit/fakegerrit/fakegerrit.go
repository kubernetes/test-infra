/*
Copyright 2022 The Kubernetes Authors.

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

package fakegerrit

import (
	"sync"

	gerrit "github.com/andygrunwald/go-gerrit"
)

type Project struct {
	Branches  map[string]*gerrit.BranchInfo
	ChangeIDs []string
}

type Change struct {
	ChangeInfo *gerrit.ChangeInfo
	Comments   map[string][]*gerrit.CommentInfo
}

type FakeGerrit struct {
	Changes  map[string]Change
	Accounts map[string]*gerrit.AccountInfo
	Projects map[string]*Project
	// lock to be thread safe
	lock sync.Mutex
}

func (fg *FakeGerrit) Reset() {
	fg.lock.Lock()
	defer fg.lock.Unlock()

	fg.Changes = make(map[string]Change)
	fg.Accounts = make(map[string]*gerrit.AccountInfo)
	fg.Projects = make(map[string]*Project)
}

func (fg *FakeGerrit) GetChangesForProject(projectName string, start int) []*gerrit.ChangeInfo {
	res := []*gerrit.ChangeInfo{}
	for _, id := range fg.Projects[projectName].ChangeIDs {
		if start > 0 {
			start--
		} else {
			res = append(res, fg.GetChange(id))
		}
	}
	return res
}

func (fg *FakeGerrit) GetComments(id string) map[string][]*gerrit.CommentInfo {
	fg.lock.Lock()
	defer fg.lock.Unlock()

	if res, ok := fg.Changes[id]; ok {
		return res.Comments
	}
	return nil
}

// Add a change to Fake gerrit and keep track that the change belongs to the given project
func (fg *FakeGerrit) AddChange(projectName string, change *gerrit.ChangeInfo) {
	fg.lock.Lock()
	defer fg.lock.Unlock()

	if project, ok := fg.Projects[projectName]; !ok {
		project = &Project{ChangeIDs: []string{}}
		project.ChangeIDs = append(project.ChangeIDs, change.ChangeID)
		fg.Projects[projectName] = project
	} else {
		project.ChangeIDs = append(project.ChangeIDs, change.ChangeID)
	}

	fg.Changes[change.ChangeID] = Change{ChangeInfo: change, Comments: make(map[string][]*gerrit.CommentInfo)}
}

func (fg *FakeGerrit) GetBranch(projectName, branchID string) *gerrit.BranchInfo {
	fg.lock.Lock()
	defer fg.lock.Unlock()
	var project *Project
	var res *gerrit.BranchInfo
	var ok bool

	if project, ok = fg.Projects[projectName]; !ok {
		return nil
	}
	if res, ok = project.Branches[branchID]; !ok {
		return nil
	}
	return res
}

func (fg *FakeGerrit) GetChange(id string) *gerrit.ChangeInfo {
	fg.lock.Lock()
	defer fg.lock.Unlock()

	if res, ok := fg.Changes[id]; ok {
		return res.ChangeInfo
	}
	return nil
}

func (fg *FakeGerrit) GetAccount(id string) *gerrit.AccountInfo {
	fg.lock.Lock()
	defer fg.lock.Unlock()

	if res, ok := fg.Accounts[id]; ok {
		return res
	}
	return nil
}

func NewFakeGerritClient() *FakeGerrit {
	return &FakeGerrit{
		Changes:  make(map[string]Change),
		Accounts: make(map[string]*gerrit.AccountInfo),
		Projects: make(map[string]*Project),
	}
}
