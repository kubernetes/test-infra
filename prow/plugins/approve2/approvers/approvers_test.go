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
	"sort"
	"testing"

	"github.com/sirupsen/logrus"

	"net/url"
	"reflect"

	"k8s.io/apimachinery/pkg/util/sets"
)

type fakeapproval struct {
	approver string
	path     string
}

func sortFiles(files []File) {
	sort.Slice(files, func(i, j int) bool {
		return files[i].String() < files[j].String()
	})
}

func TestUnapprovedFiles(t *testing.T) {
	rootApprovers := sets.NewString("Alice", "Bob")
	aApprovers := sets.NewString("Art", "Anne")
	bApprovers := sets.NewString("Bill", "Ben", "Barbara")
	cApprovers := sets.NewString("Chris", "Carol")
	dApprovers := sets.NewString("David", "Dan", "Debbie")
	eApprovers := sets.NewString("Eve", "Erin")
	edcApprovers := eApprovers.Union(dApprovers).Union(cApprovers)
	FakeRepoMap := map[string]sets.String{
		"":        rootApprovers,
		"a":       aApprovers,
		"b":       bApprovers,
		"c":       cApprovers,
		"a/d":     dApprovers,
		"a/combo": edcApprovers,
	}
	tests := []struct {
		testName           string
		filenames          []string
		currentlyApproved  sets.String
		expectedUnapproved sets.String
	}{
		{
			testName:           "Empty PR",
			filenames:          []string{},
			currentlyApproved:  sets.NewString(),
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:           "Single Root File PR Approved",
			filenames:          []string{"kubernetes.go"},
			currentlyApproved:  sets.NewString(rootApprovers.List()[0]),
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:           "Single Root File PR No One Approved",
			filenames:          []string{"kubernetes.go"},
			currentlyApproved:  sets.NewString(),
			expectedUnapproved: sets.NewString("kubernetes.go"),
		},
		{
			testName:           "B Only UnApproved",
			filenames:          []string{"b/test.go"},
			currentlyApproved:  bApprovers,
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:           "B Files Approved at Root",
			filenames:          []string{"b/test.go", "b/test_1.go"},
			currentlyApproved:  rootApprovers,
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:           "B Only UnApproved",
			filenames:          []string{"b/test_1.go", "b/test.go"},
			currentlyApproved:  sets.NewString(),
			expectedUnapproved: sets.NewString("b/test_1.go", "b/test.go"),
		},
		{
			testName:           "Combo and Other; Neither Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  sets.NewString(),
			expectedUnapproved: sets.NewString("a/combo/test.go", "a/d/test.go"),
		},
		{
			testName:           "Combo and Other; Combo Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  edcApprovers.Difference(dApprovers),
			expectedUnapproved: sets.NewString("a/d/test.go"),
		},
		{
			testName:           "Combo and Other; Both Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  edcApprovers.Intersection(dApprovers),
			expectedUnapproved: sets.NewString(),
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: TestSeed, log: logrus.WithField("plugin", "some_plugin")})
		testApprovers.RequireIssue = false
		for approver := range test.currentlyApproved {
			testApprovers.AddApprover(approver, "REFERENCE", "")
		}
		calculated := testApprovers.UnapprovedFiles()
		if !test.expectedUnapproved.Equal(calculated) {
			t.Errorf("Failed for test %v.  Expected unapproved files: %v. Found %v", test.testName, test.expectedUnapproved, calculated)
		}
	}
}

func TestUnapprovedFilesWithGranularApprovals(t *testing.T) {
	rootApprovers := sets.NewString("Alice", "Bob")
	aApprovers := sets.NewString("Art", "Anne")
	bApprovers := sets.NewString("Bill", "Ben", "Barbara")
	cApprovers := sets.NewString("Chris", "Carol")
	dApprovers := sets.NewString("David", "Dan", "Debbie")
	eApprovers := sets.NewString("Eve", "Erin")
	edcApprovers := eApprovers.Union(dApprovers).Union(cApprovers)
	FakeRepoMap := map[string]sets.String{
		"":        rootApprovers,
		"a":       aApprovers,
		"b":       bApprovers,
		"c":       cApprovers,
		"a/d":     dApprovers,
		"a/combo": edcApprovers,
	}
	tests := []struct {
		testName           string
		filenames          []string
		currentApproval    []fakeapproval
		expectedUnapproved sets.String
	}{
		{
			testName:           "Empty PR",
			filenames:          []string{},
			currentApproval:    []fakeapproval{},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:  "Single Root File PR Approved",
			filenames: []string{"kubernetes.go"},
			currentApproval: []fakeapproval{
				{approver: rootApprovers.List()[0], path: ""},
			},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:  "Single Root File PR With Single File Approved",
			filenames: []string{"kubernetes.go"},
			currentApproval: []fakeapproval{
				{approver: rootApprovers.List()[0], path: "kubernetes.go"},
			},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:           "Single Root File PR No One Approved",
			filenames:          []string{"kubernetes.go"},
			currentApproval:    []fakeapproval{},
			expectedUnapproved: sets.NewString("kubernetes.go"),
		},
		{
			testName:  "B Only UnApproved",
			filenames: []string{"b/test.go"},
			currentApproval: []fakeapproval{
				{approver: bApprovers.List()[0], path: "b/test.go"},
			},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:  "B Only Approved using wildcard",
			filenames: []string{"b/test.go", "b/test_1.go"},
			currentApproval: []fakeapproval{
				{approver: bApprovers.List()[0], path: "b/*"},
			},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:  "B Only Approved One File",
			filenames: []string{"b/test.go", "b/test_1.go"},
			currentApproval: []fakeapproval{
				{approver: bApprovers.List()[0], path: "b/test.go"},
			},
			expectedUnapproved: sets.NewString("b/test_1.go"),
		},
		{
			testName:  "B Files Approved at Root",
			filenames: []string{"b/test.go", "b/test_1.go"},
			currentApproval: []fakeapproval{
				{approver: rootApprovers.List()[0], path: "b/test.go"},
				{approver: rootApprovers.List()[0], path: "b/test_1.go"},
			},
			expectedUnapproved: sets.NewString(),
		},
		{
			testName:  "Root Approver Partially Approved Using Wildcard",
			filenames: []string{"a/test.go", "b/test.go", "c/test.go"},
			currentApproval: []fakeapproval{
				{approver: rootApprovers.List()[0], path: "a/*"},
			},
			expectedUnapproved: sets.NewString("b/test.go", "c/test.go"),
		},
		{
			testName:  "Root Approver Approver Everything Using Wildcard",
			filenames: []string{"a/test.go", "b/test.go", "c/test.go"},
			currentApproval: []fakeapproval{
				{approver: rootApprovers.List()[0], path: "*"},
			},
			expectedUnapproved: sets.NewString(),
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: TestSeed, log: logrus.WithField("plugin", "some_plugin")})
		testApprovers.RequireIssue = false
		for _, approval := range test.currentApproval {
			testApprovers.AddApprover(approval.approver, "REFERENCE", approval.path)
		}
		calculated := testApprovers.UnapprovedFiles()
		if !test.expectedUnapproved.Equal(calculated) {
			t.Errorf("Failed for test %v.  Expected unapproved files: %v. Found %v", test.testName, test.expectedUnapproved, calculated)
		}
	}
}

func TestGetFiles(t *testing.T) {
	rootApprovers := sets.NewString("Alice", "Bob")
	aApprovers := sets.NewString("Art", "Anne")
	bApprovers := sets.NewString("Bill", "Ben", "Barbara")
	cApprovers := sets.NewString("Chris", "Carol")
	dApprovers := sets.NewString("David", "Dan", "Debbie")
	eApprovers := sets.NewString("Eve", "Erin")
	edcApprovers := eApprovers.Union(dApprovers).Union(cApprovers)
	FakeRepoMap := map[string]sets.String{
		"":        rootApprovers,
		"a":       aApprovers,
		"b":       bApprovers,
		"c":       cApprovers,
		"a/d":     dApprovers,
		"a/combo": edcApprovers,
	}
	tests := []struct {
		testName          string
		filenames         []string
		currentlyApproved sets.String
		expectedFiles     []File
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: sets.NewString(),
			expectedFiles:     []File{},
		},
		{
			testName:          "Single Root File PR Approved",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(rootApprovers.List()[0]),
			expectedFiles:     []File{},
		},
		{
			testName:          "Single File PR in B No One Approved",
			filenames:         []string{"b/test.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles: []File{UnapprovedFile{
				baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
				filepath: "b",
				branch:   "master",
			}},
		},
		{
			testName:          "Single File PR in B Fully Approved",
			filenames:         []string{"b/test.go"},
			currentlyApproved: bApprovers,
			expectedFiles:     []File{},
		},
		{
			testName:          "Single Root File PR No One Approved",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles: []File{UnapprovedFile{
				baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
				filepath: "",
				branch:   "master",
			}},
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles: []File{
				UnapprovedFile{
					baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
					filepath: "a/combo",
					branch:   "master",
				},
				UnapprovedFile{
					baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
					filepath: "a/d",
					branch:   "master",
				},
			},
		},
		{
			testName:          "Combo and Other; Combo Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: eApprovers,
			expectedFiles: []File{
				UnapprovedFile{
					baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
					filepath: "a/d",
					branch:   "master",
				},
			},
		},
		{
			testName:          "Combo and Other; Both Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: edcApprovers.Intersection(dApprovers),
			expectedFiles:     []File{},
		},
		{
			testName:          "Combo, C, D; Combo and C Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go", "c/test"},
			currentlyApproved: cApprovers,
			expectedFiles: []File{
				UnapprovedFile{
					baseURL:  &url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"},
					filepath: "a/d",
					branch:   "master",
				},
			},
		},
		{
			testName:          "Files Approved Multiple times",
			filenames:         []string{"a/test.go", "a/d/test.go", "b/test"},
			currentlyApproved: rootApprovers.Union(aApprovers).Union(bApprovers),
			expectedFiles:     []File{},
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: TestSeed, log: logrus.WithField("plugin", "some_plugin")})
		testApprovers.RequireIssue = false
		for approver := range test.currentlyApproved {
			testApprovers.AddApprover(approver, "REFERENCE", "")
		}
		calculated := testApprovers.GetFiles(&url.URL{Scheme: "https", Host: "github.com", Path: "org/repo"}, "master")
		sortFiles(test.expectedFiles)
		sortFiles(calculated)
		if !reflect.DeepEqual(test.expectedFiles, calculated) {
			t.Errorf("Failed for test %v.  Expected files: %v. Found %v", test.testName, test.expectedFiles, calculated)
		}
	}
}

type approval struct {
	name string
	path string
}

func TestGetCCs(t *testing.T) {
	rootApprovers := sets.NewString("Alice", "Bob")
	aApprovers := sets.NewString("Art", "Anne")
	bApprovers := sets.NewString("Bill", "Ben", "Barbara")
	cApprovers := sets.NewString("Chris", "Carol")
	dApprovers := sets.NewString("David", "Dan", "Debbie")
	eApprovers := sets.NewString("Eve", "Erin")
	edcApprovers := eApprovers.Union(dApprovers).Union(cApprovers)
	FakeRepoMap := map[string]sets.String{
		"":        rootApprovers,
		"a":       aApprovers,
		"b":       bApprovers,
		"c":       cApprovers,
		"a/d":     dApprovers,
		"a/combo": edcApprovers,
	}
	tests := []struct {
		testName          string
		filenames         []string
		currentlyApproved []approval
		// testSeed affects who is chosen for CC
		testSeed  int64
		assignees []string
		// order matters for CCs
		expectedCCs []string
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: []approval{},
			testSeed:          0,
			expectedCCs:       []string{},
		},
		{
			testName:  "Single Root FFile PR Approved",
			filenames: []string{"kubernetes.go"},
			currentlyApproved: []approval{
				{rootApprovers.List()[0], ""},
			},
			testSeed:    13,
			expectedCCs: []string{},
		},
		{
			testName:          "Single Root File PR Unapproved Seed = 13",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: []approval{},
			testSeed:          13,
			expectedCCs:       []string{"alice"},
		},
		{
			testName:  "Single Root File PR Partially Approved Seed = 13",
			filenames: []string{"kubernetes.go", "root.go"},
			currentlyApproved: []approval{
				{"Alice", "kubernetes.go"},
			},
			testSeed:    13,
			expectedCCs: []string{"bob"},
		},
		{
			testName:          "Single Root File PR No One Seed = 10",
			filenames:         []string{"kubernetes.go"},
			testSeed:          10,
			currentlyApproved: []approval{},
			expectedCCs:       []string{"bob"},
		},
		{
			testName:  "A and B File PR. Root Partial Approval Seed = 10",
			filenames: []string{"a/test.go", "a/test2.go", "b/test.go", "b/test2.go"},
			testSeed:  10,
			currentlyApproved: []approval{
				{"Alice", "*/test2.go"},
			},
			expectedCCs: []string{"art", "barbara"},
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: []approval{},
			expectedCCs:       []string{"debbie"},
		},
		{
			testName:  "Combo and Other; Combo Approved",
			filenames: []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:  0,
			currentlyApproved: func() []approval {
				approvers := []approval{}
				for approver := range eApprovers {
					approvers = append(approvers, approval{approver, ""})
				}
				return approvers
			}(),
			expectedCCs: []string{"debbie"},
		},
		{
			testName:  "Combo and Other; Combo Partially Approved",
			filenames: []string{"a/combo/test.go", "a/combo/test2.go", "a/d/test.go", "a/d/test2.go"},
			testSeed:  0,
			currentlyApproved: func() []approval {
				approvers := []approval{}
				for approver := range eApprovers {
					approvers = append(approvers, approval{approver, "*/test2.go"})
				}
				return approvers
			}(),
			expectedCCs: []string{"debbie"},
		},
		{
			testName:  "Combo and Other; Both Partially Approved",
			filenames: []string{"a/combo/test.go", "a/combo/test2.go", "a/d/test.go", "a/d/test2.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"Dan", "a/d/test2.go"},
				{"Eve", "*/test2.go"},
			},
			expectedCCs: []string{"debbie"},
		},
		{
			testName:  "Combo and Other; Both Partially Approved, Seed = 10",
			filenames: []string{"a/combo/test.go", "a/combo/test2.go", "a/d/test.go", "a/d/test2.go"},
			testSeed:  5,
			currentlyApproved: []approval{
				{"Dan", "a/d/test2.go"},
				{"Eve", "*/test2.go"},
			},
			expectedCCs: []string{"david"},
		},
		{
			testName:  "Combo and Other; Both Approved",
			filenames: []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:  0,
			currentlyApproved: func() []approval {
				approvers := []approval{}
				for approver := range dApprovers {
					approvers = append(approvers, approval{approver, ""})
				}
				return approvers
			}(), // dApprovers can approve combo and d directory
			expectedCCs: []string{},
		},
		{
			testName:          "Combo, C, D; None Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: []approval{},
			// chris can approve c and combo, debbie can approve d
			expectedCCs: []string{"carol", "debbie"},
		},
		{
			testName:          "A, B, C; Nothing Approved",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: []approval{},
			// Need an approver from each of the three owners files
			expectedCCs: []string{"anne", "bill", "carol"},
		},
		{
			testName:  "A, B, C; One File Approved Per Folder By Root Approvers",
			filenames: []string{"a/test.go", "a/test2.go", "b/test.go", "b/test2.go", "c/test.go", "c/test2.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"Alice", "a/test.go"},
				{"Alice", "b/test.go"},
				{"Bob", "c/test.go"},
			},
			expectedCCs: []string{"anne", "bill", "carol"},
		},
		{
			testName:  "A, B, C; Partial Files Approved By Root Approver",
			filenames: []string{"a/test.go", "b/test.go", "b/test2.go", "c/test.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"Alice", "a/*"},
				{"Alice", "*/test2.go"},
			},
			// Need an approver from each of the three owners files
			expectedCCs: []string{"bill", "carol"},
		},
		{
			testName:  "A, B, C; Partially approved by non-suggested approvers",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			// Approvers are valid approvers, but not the one we would suggest
			currentlyApproved: []approval{
				{"Art", ""},
				{"Ben", ""},
			},
			// We don't suggest approvers for a and b, only for unapproved c.
			expectedCCs: []string{"carol"},
		},
		{
			testName:  "A, B, C; Nothing approved, but assignees can approve",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			// Approvers are valid approvers, but not the one we would suggest
			currentlyApproved: []approval{},
			assignees:         []string{"Art", "Ben"},
			// We suggest assigned people rather than "suggested" people
			// Suggested would be "Anne", "Bill", "Carol" if no one was assigned.
			expectedCCs: []string{"art", "ben", "carol"},
		},
		{
			testName:          "A, B, C; Nothing approved, but SOME assignees can approve",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: []approval{},
			// Assignees are a mix of potential approvers and random people
			assignees: []string{"Art", "Ben", "John", "Jack"},
			// We suggest assigned people rather than "suggested" people
			expectedCCs: []string{"art", "ben", "carol"},
		},
		{
			testName:          "Assignee is top OWNER, No one has approved",
			filenames:         []string{"a/test.go"},
			testSeed:          0,
			currentlyApproved: []approval{},
			// Assignee is a root approver
			assignees:   []string{"alice"},
			expectedCCs: []string{"alice"},
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: test.testSeed, log: logrus.WithField("plugin", "some_plugin")})
		testApprovers.RequireIssue = false
		for _, aprvl := range test.currentlyApproved {
			testApprovers.AddApprover(aprvl.name, "REFERENCE", aprvl.path)
		}
		testApprovers.AddAssignees(test.assignees...)
		calculated := testApprovers.GetCCs()
		if !reflect.DeepEqual(test.expectedCCs, calculated) {
			t.Errorf("Failed for test %v.  Expected CCs: %v. Found %v", test.testName, test.expectedCCs, calculated)
		}
	}
}

func TestIsApproved(t *testing.T) {
	rootApprovers := sets.NewString("Alice", "Bob")
	aApprovers := sets.NewString("Art", "Anne")
	bApprovers := sets.NewString("Bill", "Ben", "Barbara")
	cApprovers := sets.NewString("Chris", "Carol")
	dApprovers := sets.NewString("David", "Dan", "Debbie")
	eApprovers := sets.NewString("Eve", "Erin")
	edcApprovers := eApprovers.Union(dApprovers).Union(cApprovers)
	FakeRepoMap := map[string]sets.String{
		"":        rootApprovers,
		"a":       aApprovers,
		"b":       bApprovers,
		"c":       cApprovers,
		"a/d":     dApprovers,
		"a/combo": edcApprovers,
	}
	tests := []struct {
		testName          string
		filenames         []string
		currentlyApproved []approval
		testSeed          int64
		isApproved        bool
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: []approval{},
			testSeed:          0,
			isApproved:        false,
		},
		{
			testName:  "Single Root File PR Approved",
			filenames: []string{"kubernetes.go"},
			currentlyApproved: []approval{
				{rootApprovers.List()[0], ""},
			},
			testSeed:   3,
			isApproved: true,
		},
		{
			testName:          "Single Root File PR No One Approved",
			filenames:         []string{"kubernetes.go"},
			testSeed:          0,
			currentlyApproved: []approval{},
			isApproved:        false,
		},
		{
			testName:  "Multiple Root Files. Wildcard Apporval By Root Approver",
			filenames: []string{"kubernetes.go", "root.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{rootApprovers.List()[0], "*"},
			},
			isApproved: true,
		},
		{
			testName:  "Multiple Root Files. Partial Apporval By Root Approver",
			filenames: []string{"kubernetes.go", "root.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{rootApprovers.List()[0], "root.go"},
			},
			isApproved: false,
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: []approval{},
			isApproved:        false,
		},
		{
			testName:  "Combo and Other; Both Approved",
			filenames: []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"David", ""},
				{"Eve", ""},
			},
			isApproved: true,
		},
		{
			testName:  "Combo and Other; Both Partially Approved",
			filenames: []string{"a/combo/test.go", "a/combo/test2.go", "a/d/test.go", "a/d/test2.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"David", "*/test2.go"},
				{"Eve", "*/test2.go"},
			},
			isApproved: false,
		},
		{
			testName:          "A, B, C; Nothing Approved",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: []approval{},
			isApproved:        false,
		},
		{
			testName:  "A, B, C; A and B Approved",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			currentlyApproved: []approval{
				{aApprovers.List()[0], ""},
				{bApprovers.List()[0], ""},
			},
			isApproved: false,
		},
		{
			testName:  "A, B, C; Approved At the Root",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			currentlyApproved: []approval{
				{rootApprovers.List()[0], ""},
			},
			isApproved: true,
		},
		{
			testName:  "A, B, C; Partially Approved At the Root",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			currentlyApproved: []approval{
				{rootApprovers.List()[0], "a/*"},
				{rootApprovers.List()[0], "b/*"},
			},
			isApproved: false,
		},
		{
			testName:  "A, B, C; Approved At the Leaves",
			filenames: []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"Anne", ""},
				{"Ben", ""},
				{"Carol", ""},
			},
			isApproved: true,
		},
		{
			testName:  "A, B, C; Partially By Root Approver And Partially By Level",
			filenames: []string{"a/test.go", "a/test2.go", "b/test.go", "b/test2.go", "c/test.go", "c/test2.go"},
			testSeed:  0,
			currentlyApproved: []approval{
				{"Alice", "*/test2.go"},
				{"Anne", ""},
				{"Ben", ""},
				{"Carol", ""},
			},
			isApproved: true,
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: test.testSeed, log: logrus.WithField("plugin", "some_plugin")})
		for _, approver := range test.currentlyApproved {
			testApprovers.AddApprover(approver.name, "REFERENCE", approver.path)
		}
		calculated := testApprovers.IsApproved()
		if test.isApproved != calculated {
			t.Errorf("Failed for test %v.  Expected Approval Status: %v. Found %v", test.testName, test.isApproved, calculated)
		}
	}
}

func TestIsApprovedWithIssue(t *testing.T) {
	aApprovers := sets.NewString("Author", "Anne", "Carl")
	bApprovers := sets.NewString("Bill", "Carl")
	FakeRepoMap := map[string]sets.String{"a": aApprovers, "b": bApprovers}
	tests := []struct {
		testName          string
		filenames         []string
		currentlyApproved []approval
		noIssueApprovers  sets.String
		associatedIssue   int
		useselfApprove    bool
		isApproved        bool
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			isApproved:        false,
		},
		{
			testName:          "Single. No issue. No Approval",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			isApproved:        false,
		},
		{
			testName:          "Single file. No issue approval. File approved",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{{"Carl", ""}},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			isApproved:        false,
		},
		{
			testName:          "Single file. With issue. File approved. Issue not approved",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{{"Carl", ""}},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   100,
			isApproved:        true,
		},
		{
			testName:          "Single file. With issue. File approved. Issue Approved",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{{"Carl", ""}},
			noIssueApprovers:  sets.NewString("Carl"),
			associatedIssue:   100,
			isApproved:        true,
		},
		{
			testName:          "Single file. With issue. File not approved. Issue approved",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString("Carl"),
			associatedIssue:   100,
			isApproved:        false,
		},
		{
			testName:          "Single file. With issue. File not approved. Issue not approved",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   100,
			isApproved:        false,
		},
		{
			testName:          "Single file. With issue. File approved. Issue approval not valid",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{{"Anne", ""}},
			noIssueApprovers:  sets.NewString("Bill"),
			associatedIssue:   100,
			isApproved:        true,
		},
		{
			testName:          "Two files. No Issue. Files partially approved. Issue Approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{{"Carl", "a/*"}},
			noIssueApprovers:  sets.NewString("Bill"),
			associatedIssue:   0,
			isApproved:        false,
		},
		{
			testName:          "Two files. With Issue. Files partially approved. Issue Approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{{"Carl", "a/*"}},
			noIssueApprovers:  sets.NewString("Bill"),
			associatedIssue:   100,
			isApproved:        false,
		},
		{
			testName:          "Two files. No Issue. Files Approved. Issue not approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{{"Carl", ""}},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			isApproved:        false,
		},
		{
			testName:          "Two files. With Issue. Files not approved. Issue approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString("Anne"),
			associatedIssue:   100,
			isApproved:        false,
		},
		{
			testName:  "Two files. With Issue. Files approved. Issue approved",
			filenames: []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{
				{"Anne", ""},
				{"Bill", ""},
			},
			noIssueApprovers: sets.NewString("Anne"),
			associatedIssue:  100,
			isApproved:       true,
		},
		{
			testName:          "Self approval missing issue",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			useselfApprove:    true,
			isApproved:        true,
		},
		{
			testName:          "Self approval with issue",
			filenames:         []string{"a/file.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   10,
			useselfApprove:    true,
			isApproved:        true,
		},
		{
			testName:          "Self approval. No issue. Issue not approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			useselfApprove:    true,
			isApproved:        false,
		},
		{
			testName:          "Self approval. With issues. Files partially approved. Issue not approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{},
			noIssueApprovers:  sets.NewString(),
			associatedIssue:   0,
			useselfApprove:    true,
			isApproved:        false,
		},
		{
			testName:          "Self approval. With Issue. Files approved. Issue approved",
			filenames:         []string{"a/file.go", "b/file2.go"},
			currentlyApproved: []approval{{"Bill", ""}},
			noIssueApprovers:  sets.NewString("Carl"),
			associatedIssue:   100,
			useselfApprove:    true,
			isApproved:        true,
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: 0, log: logrus.WithField("plugin", "some_plugin")})
		testApprovers.RequireIssue = true
		testApprovers.AssociatedIssue = test.associatedIssue
		for _, aprvl := range test.currentlyApproved {
			testApprovers.AddApprover(aprvl.name, "REFERENCE", aprvl.path)
		}
		if test.useselfApprove {
			testApprovers.AddAuthorSelfApprover("Author", "REFERENCE", "")
		}
		for nia := range test.noIssueApprovers {
			testApprovers.AddNoIssueApprover(nia, "REFERENCE")
		}
		if test.useselfApprove {
			testApprovers.AddNoIssueAuthorSelfApprover("Author", "REFERENCE")
		}
		calculated := testApprovers.IsApproved()
		if test.isApproved != calculated {
			t.Errorf("Failed for test %v.  Expected Approval Status: %v. Found %v", test.testName, test.isApproved, calculated)
		}
	}
}

func TestGetFilesApprovers(t *testing.T) {
	tests := []struct {
		testName       string
		filenames      []string
		approvals      []approval
		owners         map[string]sets.String
		expectedStatus map[string]sets.String
	}{
		{
			testName:       "Empty PR",
			filenames:      []string{},
			approvals:      []approval{},
			owners:         map[string]sets.String{},
			expectedStatus: map[string]sets.String{},
		},
		{
			testName:  "No approvals",
			filenames: []string{"a/a", "c"},
			approvals: []approval{},
			owners:    map[string]sets.String{"": sets.NewString("RootOwner")},
			expectedStatus: map[string]sets.String{
				"a/a": sets.NewString(),
				"c":   sets.NewString(),
			},
		},
		{
			testName:  "Partial approvals",
			filenames: []string{"a/a", "b/b", "c/c"},
			approvals: []approval{
				{"AApprover", "a/*"},
				{"BApprover", "a/*"},
			},
			owners: map[string]sets.String{
				"":  sets.NewString("RootOwner"),
				"a": sets.NewString("AApprover"),
				"b": sets.NewString("BApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"a/a": sets.NewString("aapprover"),
				"b/b": sets.NewString(),
				"c/c": sets.NewString(),
			},
		},
		{
			testName: "Approvers approves some",
			filenames: []string{
				"a/a",
				"c/c",
			},
			approvals: []approval{{"CApprover", ""}},
			owners: map[string]sets.String{
				"a": sets.NewString("AApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"a/a": sets.NewString(),
				"c/c": sets.NewString("capprover"),
			},
		},
		{
			testName: "Multiple approvers",
			filenames: []string{
				"a/a",
				"c/c",
			},
			approvals: []approval{
				{"RootApprover", ""},
				{"CApprover", ""},
			},
			owners: map[string]sets.String{
				"":  sets.NewString("RootApprover"),
				"a": sets.NewString("AApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"a/a": sets.NewString("rootapprover"),
				"c/c": sets.NewString("rootapprover", "capprover"),
			},
		},
		{
			testName: "Multiple approvers using path approvals",
			filenames: []string{
				"root",
				"a/a",
				"b/b",
				"c/c",
			},
			approvals: []approval{
				{"RootApprover", ""},
				{"BApprover", "b/*"},
				{"CApprover", "b/*"},
				{"CApprover", ""},
			},
			owners: map[string]sets.String{
				"":  sets.NewString("RootApprover"),
				"a": sets.NewString("AApprover"),
				"b": sets.NewString("BApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"root": sets.NewString("rootapprover"),
				"a/a":  sets.NewString("rootapprover"),
				"b/b":  sets.NewString("rootapprover", "bapprover"),
				"c/c":  sets.NewString("rootapprover", "capprover"),
			},
		},
		{
			testName:       "Case-insensitive approvers",
			filenames:      []string{"file"},
			approvals:      []approval{{"RootApprover", ""}},
			owners:         map[string]sets.String{"": sets.NewString("RootApprover")},
			expectedStatus: map[string]sets.String{"file": sets.NewString("rootapprover")},
		},
	}

	for _, test := range tests {
		testApprovers := NewApprovers(Owners{filenames: test.filenames, repo: createFakeRepo(test.owners), log: logrus.WithField("plugin", "some_plugin")})
		for _, aprvl := range test.approvals {
			testApprovers.AddApprover(aprvl.name, "REFERENCE", aprvl.path)
		}
		calculated := testApprovers.GetFilesApprovers()
		if !reflect.DeepEqual(test.expectedStatus, calculated) {
			t.Errorf("Failed for test %v.  Expected approval status: %v. Found %v", test.testName, test.expectedStatus, calculated)
		}
	}
}

func TestGetMessage(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AddApprover("Bill", "REFERENCE", "")

	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Bill</a>*
To complete the [pull request process](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process), please assign **alice**
You can assign the PR to them by writing ` + "`/assign @alice`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

Out of **2** files: **1** are approved and **1** are unapproved.  

Needs approval from approvers in these files:
- **[a/OWNERS](https://github.com/org/repo/blob/dev/a/OWNERS)**


Approvers can indicate their approval by writing ` + "`/approve2`" + ` in a comment
Approvers can also choose to approve only specific files by writing ` + "`/approve2 files <path-to-file>`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve2 cancel`" + ` in a comment
The status of the PR is:  

- **[a/](https://github.com/org/repo/blob/dev/a)**
- ~~[b/](https://github.com/org/repo/blob/dev/b)~~ (approved) [bill]


<!-- META={"approvers":["alice"]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "dev"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessagePartiallyApproved(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b1.go", "b/b2.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AddApprover("Bill", "REFERENCE", "b/b1.go")

	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Bill</a>*
To complete the [pull request process](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process), please assign **alice**
You can assign the PR to them by writing ` + "`/assign @alice`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

Out of **3** files: **1** are approved and **2** are unapproved.  

Needs approval from approvers in these files:
- **[a/OWNERS](https://github.com/org/repo/blob/dev/a/OWNERS)**
- **[b/OWNERS](https://github.com/org/repo/blob/dev/b/OWNERS)**


Approvers can indicate their approval by writing ` + "`/approve2`" + ` in a comment
Approvers can also choose to approve only specific files by writing ` + "`/approve2 files <path-to-file>`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve2 cancel`" + ` in a comment
The status of the PR is:  

- **[a/](https://github.com/org/repo/blob/dev/a)**
- **[b/](https://github.com/org/repo/blob/dev/b)** (partially approved, need additional approvals) [bill]


<!-- META={"approvers":["alice"]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "dev"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageAllApproved(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AddApprover("Alice", "REFERENCE", "")
	ap.AddLGTMer("Bill", "REFERENCE", "")
	ap.AddNoIssueApprover("Alice", "REFERENCE")

	want := `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Alice</a>*, *<a href="REFERENCE" title="LGTM">Bill</a>*

Associated issue requirement bypassed by: *<a href="REFERENCE" title="Approved">Alice</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

Out of **2** files: **2** are approved and **0** are unapproved.  

The status of the PR is:  

- ~~[a/](https://github.com/org/repo/blob/master/a)~~ (approved) [alice]
- ~~[b/](https://github.com/org/repo/blob/master/b)~~ (approved) [bill]


<!-- META={"approvers":[]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageFilesApprovedIssueNotApproved(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AddApprover("Alice", "REFERENCE", "")
	ap.AddLGTMer("Bill", "REFERENCE", "")

	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Alice</a>*, *<a href="REFERENCE" title="LGTM">Bill</a>*

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

Out of **2** files: **2** are approved and **0** are unapproved.  

The status of the PR is:  

- ~~[a/](https://github.com/org/repo/blob/master/a)~~ (approved) [alice]
- ~~[b/](https://github.com/org/repo/blob/master/b)~~ (approved) [bill]


<!-- META={"approvers":[]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageNoneApproved(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.AddAuthorSelfApprover("John", "REFERENCE", "")
	ap.RequireIssue = true
	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Author self-approved">John</a>*
To complete the [pull request process](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process), please assign **alice**, **bill**
You can assign the PR to them by writing ` + "`/assign @alice @bill`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

Out of **2** files: **0** are approved and **2** are unapproved.  

Needs approval from approvers in these files:
- **[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)**
- **[b/OWNERS](https://github.com/org/repo/blob/master/b/OWNERS)**


Approvers can indicate their approval by writing ` + "`/approve2`" + ` in a comment
Approvers can also choose to approve only specific files by writing ` + "`/approve2 files <path-to-file>`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve2 cancel`" + ` in a comment
The status of the PR is:  

- **[a/](https://github.com/org/repo/blob/master/a)**
- **[b/](https://github.com/org/repo/blob/master/b)**


<!-- META={"approvers":["alice","bill"]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageApprovedIssueAssociated(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.go"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AssociatedIssue = 12345
	ap.AddAuthorSelfApprover("John", "REFERENCE", "")
	ap.AddApprover("Bill", "REFERENCE", "")
	ap.AddApprover("Alice", "REFERENCE", "")

	want := `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Alice</a>*, *<a href="REFERENCE" title="Approved">Bill</a>*, *<a href="REFERENCE" title="Author self-approved">John</a>*

Associated issue: *#12345*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

Out of **2** files: **2** are approved and **0** are unapproved.  

The status of the PR is:  

- ~~[a/](https://github.com/org/repo/blob/master/a)~~ (approved) [alice]
- ~~[b/](https://github.com/org/repo/blob/master/b)~~ (approved) [bill]


<!-- META={"approvers":[]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageApprovedNoIssueByPassed(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/b.md"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.RequireIssue = true
	ap.AddAuthorSelfApprover("John", "REFERENCE", "")
	ap.AddApprover("Bill", "REFERENCE", "")
	ap.AddNoIssueApprover("Bill", "REFERENCE")
	ap.AddApprover("Alice", "REFERENCE", "")
	ap.AddNoIssueApprover("Alice", "REFERENCE")

	want := `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Approved">Alice</a>*, *<a href="REFERENCE" title="Approved">Bill</a>*, *<a href="REFERENCE" title="Author self-approved">John</a>*

Associated issue requirement bypassed by: *<a href="REFERENCE" title="Approved">Alice</a>*, *<a href="REFERENCE" title="Approved">Bill</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

Out of **2** files: **2** are approved and **0** are unapproved.  

The status of the PR is:  

- ~~[a/](https://github.com/org/repo/blob/master/a)~~ (approved) [alice]
- ~~[b/](https://github.com/org/repo/blob/master/b)~~ (approved) [bill]


<!-- META={"approvers":[]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageMDOwners(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/README.md"},
			repo: createFakeRepo(map[string]sets.String{
				"a":           sets.NewString("Alice"),
				"b":           sets.NewString("Bill"),
				"b/README.md": sets.NewString("Doctor"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.AddAuthorSelfApprover("John", "REFERENCE", "")
	ap.RequireIssue = true
	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Author self-approved">John</a>*
To complete the [pull request process](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process), please assign **alice**, **doctor**
You can assign the PR to them by writing ` + "`/assign @alice @doctor`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

Out of **2** files: **0** are approved and **2** are unapproved.  

Needs approval from approvers in these files:
- **[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)**
- **[b/README.md](https://github.com/org/repo/blob/master/b/README.md)**


Approvers can indicate their approval by writing ` + "`/approve2`" + ` in a comment
Approvers can also choose to approve only specific files by writing ` + "`/approve2 files <path-to-file>`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve2 cancel`" + ` in a comment
The status of the PR is:  

- **[a/](https://github.com/org/repo/blob/master/a)**
- **[b/README.md/](https://github.com/org/repo/blob/master/b/README.md)**


<!-- META={"approvers":["alice","doctor"]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}

func TestGetMessageDifferentGitHubLink(t *testing.T) {
	ap := NewApprovers(
		Owners{
			filenames: []string{"a/a.go", "b/README.md"},
			repo: createFakeRepo(map[string]sets.String{
				"a": sets.NewString("Alice"),
				"b": sets.NewString("Bill", "Doctor"),
			}),
			log: logrus.WithField("plugin", "some_plugin"),
		},
	)
	ap.AddAuthorSelfApprover("John", "REFERENCE", "")
	ap.RequireIssue = true
	want := `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="REFERENCE" title="Author self-approved">John</a>*
To complete the [pull request process](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process), please assign **alice**, **bill**
You can assign the PR to them by writing ` + "`/assign @alice @bill`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve2 no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands?repo=org%2Frepo).

Out of **2** files: **0** are approved and **2** are unapproved.  

Needs approval from approvers in these files:
- **[a/OWNERS](https://github.mycorp.com/org/repo/blob/master/a/OWNERS)**
- **[b/OWNERS](https://github.mycorp.com/org/repo/blob/master/b/OWNERS)**


Approvers can indicate their approval by writing ` + "`/approve2`" + ` in a comment
Approvers can also choose to approve only specific files by writing ` + "`/approve2 files <path-to-file>`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve2 cancel`" + ` in a comment
The status of the PR is:  

- **[a/](https://github.mycorp.com/org/repo/blob/master/a)**
- **[b/](https://github.mycorp.com/org/repo/blob/master/b)**


<!-- META={"approvers":["alice","bill"]} -->`
	if got := GetMessage(ap, &url.URL{Scheme: "https", Host: "github.mycorp.com"}, "https://go.k8s.io/bot-commands", "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process", "org", "repo", "master"); got == nil {
		t.Error("GetMessage() failed")
	} else if *got != want {
		t.Errorf("GetMessage() = %+v, want = %+v", *got, want)
	}
}
