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
	"testing"

	"fmt"
	"reflect"

	"k8s.io/kubernetes/pkg/util/sets"
)

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
			expectedUnapproved: sets.NewString(""),
		},
		{
			testName:           "B Only UnApproved",
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
			expectedUnapproved: sets.NewString("b"),
		},
		{
			testName:           "Combo and Other; Neither Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  sets.NewString(),
			expectedUnapproved: sets.NewString("a/combo", "a/d"),
		},
		{
			testName:           "Combo and Other; Combo Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  edcApprovers.Difference(dApprovers),
			expectedUnapproved: sets.NewString("a/d"),
		},
		{
			testName:           "Combo and Other; Both Approved",
			filenames:          []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved:  edcApprovers.Intersection(dApprovers),
			expectedUnapproved: sets.NewString(),
		},
	}

	for _, test := range tests {
		testApprovers := Approvers{Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: TEST_SEED}, test.currentlyApproved}
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
			expectedFiles:     []File{ApprovedFile{"", sets.NewString(rootApprovers.List()[0])}},
		},
		{
			testName:          "Single File PR in B No One Approved",
			filenames:         []string{"b/test.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles:     []File{UnapprovedFile{"b"}},
		},
		{
			testName:          "Single File PR in B Fully Approved",
			filenames:         []string{"b/test.go"},
			currentlyApproved: bApprovers,
			expectedFiles:     []File{ApprovedFile{"b", bApprovers}},
		},
		{
			testName:          "Single Root File PR No One Approved",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles:     []File{UnapprovedFile{""}},
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: sets.NewString(),
			expectedFiles:     []File{UnapprovedFile{"a/combo"}, UnapprovedFile{"a/d"}},
		},
		{
			testName:          "Combo and Other; Combo Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: eApprovers,
			expectedFiles:     []File{ApprovedFile{"a/combo", eApprovers}, UnapprovedFile{"a/d"}},
		},
		{
			testName:          "Combo and Other; Both Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			currentlyApproved: edcApprovers.Intersection(dApprovers),
			expectedFiles:     []File{ApprovedFile{"a/combo", edcApprovers.Intersection(dApprovers)}, ApprovedFile{"a/d", edcApprovers.Intersection(dApprovers)}},
		},
		{
			testName:          "Combo, C, D; Combo and C Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go", "c/test"},
			currentlyApproved: cApprovers,
			expectedFiles:     []File{ApprovedFile{"a/combo", cApprovers}, UnapprovedFile{"a/d"}, ApprovedFile{"c", cApprovers}},
		},
		{
			testName:          "Files Approved Multiple times",
			filenames:         []string{"a/test.go", "a/d/test.go", "b/test"},
			currentlyApproved: rootApprovers.Union(aApprovers).Union(bApprovers),
			expectedFiles:     []File{ApprovedFile{"a", rootApprovers.Union(aApprovers)}, ApprovedFile{"b", rootApprovers.Union(bApprovers)}},
		},
	}

	for _, test := range tests {
		testApprovers := Approvers{Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: TEST_SEED}, test.currentlyApproved}
		calculated := testApprovers.GetFiles()
		if !reflect.DeepEqual(test.expectedFiles, calculated) {
			t.Errorf("Failed for test %v.  Expected files: %v. Found %v", test.testName, test.expectedFiles, calculated)
		}
	}
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
		currentlyApproved sets.String
		// testSeed affects who is chosen for CC
		testSeed int64
		// order matters for CCs
		expectedCCs []string
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: sets.NewString(),
			testSeed:          0,
			expectedCCs:       []string{},
		},
		{
			testName:          "Single Root FFile PR Approved",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(rootApprovers.List()[0]),
			testSeed:          13,
			expectedCCs:       []string{},
		},
		{
			testName:          "Single Root File PR Unapproved Seed = 13",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(),
			testSeed:          13,
			expectedCCs:       []string{"Alice"},
		},
		{
			testName:          "Single Root File PR No One Seed = 10",
			filenames:         []string{"kubernetes.go"},
			testSeed:          10,
			currentlyApproved: sets.NewString(),
			expectedCCs:       []string{"Bob"},
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			expectedCCs:       []string{"Dan"},
		},
		{
			testName:          "Combo and Other; Combo Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: eApprovers,
			expectedCCs:       []string{"Dan"},
		},
		{
			testName:          "Combo and Other; Both Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: dApprovers, // dApprovers can approve combo and d directory
			expectedCCs:       []string{},
		},
		{
			testName:          "Combo, C, D; None Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			// chris can approve c and combo, debbie can approve d
			expectedCCs: []string{"Chris", "Debbie"},
		},
		{
			testName:          "A, B, C; Nothing Approved",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			// Need an approver from each of the three owners files
			expectedCCs: []string{"Anne", "Bill", "Carol"},
		},
	}

	for _, test := range tests {
		testApprovers := Approvers{Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: test.testSeed}, test.currentlyApproved}
		calculated := testApprovers.GetCCs()
		if !reflect.DeepEqual(test.expectedCCs, calculated) {
			fmt.Printf("Currently Approved %v\n", test.currentlyApproved)
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
		currentlyApproved sets.String
		testSeed          int64
		isApproved        bool
	}{
		{
			testName:          "Empty PR",
			filenames:         []string{},
			currentlyApproved: sets.NewString(),
			testSeed:          0,
			isApproved:        true,
		},
		{
			testName:          "Single Root File PR Approved",
			filenames:         []string{"kubernetes.go"},
			currentlyApproved: sets.NewString(rootApprovers.List()[0]),
			testSeed:          3,
			isApproved:        true,
		},
		{
			testName:          "Single Root File PR No One Approved",
			filenames:         []string{"kubernetes.go"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			isApproved:        false,
		},
		{
			testName:          "Combo and Other; Neither Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			isApproved:        false,
		},
		{
			testName:          "Combo and Other; Both Approved",
			filenames:         []string{"a/combo/test.go", "a/d/test.go"},
			testSeed:          0,
			currentlyApproved: edcApprovers.Intersection(dApprovers),
			isApproved:        true,
		},
		{
			testName:          "A, B, C; Nothing Approved",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: sets.NewString(),
			isApproved:        false,
		},
		{
			testName:          "A, B, C; Partially Approved",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: aApprovers.Union(bApprovers),
			isApproved:        false,
		},
		{
			testName:          "A, B, C; Approved At the Root",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: rootApprovers,
			isApproved:        true,
		},
		{
			testName:          "A, B, C; Approved At the Leaves",
			filenames:         []string{"a/test.go", "b/test.go", "c/test"},
			testSeed:          0,
			currentlyApproved: sets.NewString("Anne", "Ben", "Carol"),
			isApproved:        true,
		},
	}

	for _, test := range tests {
		testApprovers := Approvers{Owners{filenames: test.filenames, repo: createFakeRepo(FakeRepoMap), seed: test.testSeed}, test.currentlyApproved}
		calculated := testApprovers.IsApproved()
		if test.isApproved != calculated {
			fmt.Printf("Currently Approved %v\n", test.currentlyApproved)
			t.Errorf("Failed for test %v.  Expected Approval Status: %v. Found %v", test.testName, test.isApproved, calculated)
		}
	}
}

func TestGetFilesApprovers(t *testing.T) {
	tests := []struct {
		testName       string
		filenames      []string
		approvers      []string
		owners         map[string]sets.String
		expectedStatus map[string]sets.String
	}{
		{
			testName:       "Empty PR",
			filenames:      []string{},
			approvers:      []string{},
			owners:         map[string]sets.String{},
			expectedStatus: map[string]sets.String{},
		},
		{
			testName:       "No approvers",
			filenames:      []string{"a/a", "c"},
			approvers:      []string{},
			owners:         map[string]sets.String{"": sets.NewString("RootOwner")},
			expectedStatus: map[string]sets.String{"": sets.String{}},
		},
		{
			testName: "Approvers approves some",
			filenames: []string{
				"a/a",
				"c/c",
			},
			approvers: []string{"CApprover"},
			owners: map[string]sets.String{
				"a": sets.NewString("AApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"a": sets.String{},
				"c": sets.NewString("CApprover"),
			},
		},
		{
			testName: "Multiple approvers",
			filenames: []string{
				"a/a",
				"c/c",
			},
			approvers: []string{"RootApprover", "CApprover"},
			owners: map[string]sets.String{
				"":  sets.NewString("RootApprover"),
				"a": sets.NewString("AApprover"),
				"c": sets.NewString("CApprover"),
			},
			expectedStatus: map[string]sets.String{
				"a": sets.NewString("RootApprover"),
				"c": sets.NewString("RootApprover", "CApprover"),
			},
		},
		{
			testName:       "Case-sensitive approvers",
			filenames:      []string{"file"},
			approvers:      []string{"RootApprover"},
			owners:         map[string]sets.String{"": sets.NewString("rootapprover")},
			expectedStatus: map[string]sets.String{"": sets.NewString()},
		},
	}

	for _, test := range tests {
		testApprovers := Approvers{Owners{filenames: test.filenames, repo: createFakeRepo(test.owners)}, sets.NewString(test.approvers...)}
		calculated := testApprovers.GetFilesApprovers()
		if !reflect.DeepEqual(test.expectedStatus, calculated) {
			t.Errorf("Failed for test %v.  Expected approval status: %v. Found %v", test.testName, test.expectedStatus, calculated)
		}
	}
}
