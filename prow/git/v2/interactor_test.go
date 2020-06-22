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

package git

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestInteractor_Clone(t *testing.T) {
	var testCases = []struct {
		name          string
		dir           string
		from          string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			dir:  "/else",
			from: "/somewhere",
			responses: map[string]execResponse{
				"clone /somewhere /else": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"clone", "/somewhere", "/else"},
			},
			expectedErr: false,
		},
		{
			name: "clone fails",
			dir:  "/else",
			from: "/somewhere",
			responses: map[string]execResponse{
				"clone /somewhere /else": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"clone", "/somewhere", "/else"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				dir:      testCase.dir,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.Clone(testCase.from)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_MirrorClone(t *testing.T) {
	var testCases = []struct {
		name          string
		dir           string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			dir:  "/else",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"clone --mirror someone.com /else": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"clone", "--mirror", "someone.com", "/else"},
			},
			expectedErr: false,
		},
		{
			name: "remote resolution fails",
			dir:  "/else",
			remote: func() (string, error) {
				return "", errors.New("oops")
			},
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
		{
			name: "clone fails",
			dir:  "/else",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"clone --mirror someone.com /else": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"clone", "--mirror", "someone.com", "/else"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				dir:      testCase.dir,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.MirrorClone()
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Checkout(t *testing.T) {
	var testCases = []struct {
		name          string
		commitlike    string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:       "happy case",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"checkout shasum": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "shasum"},
			},
			expectedErr: false,
		},
		{
			name:       "checkout fails",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"checkout shasum": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "shasum"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.Checkout(testCase.commitlike)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_RevParse(t *testing.T) {
	var testCases = []struct {
		name          string
		commitlike    string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedOut   string
		expectedErr   bool
	}{
		{
			name:       "happy case",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"rev-parse shasum": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"rev-parse", "shasum"},
			},
			expectedOut: "ok",
			expectedErr: false,
		},
		{
			name:       "rev-parse fails",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"rev-parse shasum": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"rev-parse", "shasum"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualOut, actualErr := i.RevParse(testCase.commitlike)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
			if actualOut != testCase.expectedOut {
				t.Errorf("%s: got incorrect output: expected %v, got %v", testCase.name, testCase.expectedOut, actualOut)
			}
		})
	}
}

func TestInteractor_BranchExists(t *testing.T) {
	var testCases = []struct {
		name          string
		branch        string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedOut   bool
	}{
		{
			name:   "happy case",
			branch: "branch",
			responses: map[string]execResponse{
				"ls-remote --exit-code --heads origin branch": {
					out: []byte(`c165713776618ff3162643ea4d0382ca039adfeb	refs/heads/branch`),
				},
			},
			expectedCalls: [][]string{
				{"ls-remote", "--exit-code", "--heads", "origin", "branch"},
			},
			expectedOut: true,
		},
		{
			name:   "ls-remote fails",
			branch: "branch",
			responses: map[string]execResponse{
				"ls-remote --exit-code --heads origin branch": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"ls-remote", "--exit-code", "--heads", "origin", "branch"},
			},
			expectedOut: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualOut := i.BranchExists(testCase.branch)
			if testCase.expectedOut != actualOut {
				t.Errorf("%s: got incorrect output: expected %v, got %v", testCase.name, testCase.expectedOut, actualOut)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_CheckoutNewBranch(t *testing.T) {
	var testCases = []struct {
		name          string
		branch        string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:   "happy case",
			branch: "new-branch",
			responses: map[string]execResponse{
				"checkout -b new-branch": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "-b", "new-branch"},
			},
			expectedErr: false,
		},
		{
			name:   "checkout fails",
			branch: "new-branch",
			responses: map[string]execResponse{
				"checkout -b new-branch": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "-b", "new-branch"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.CheckoutNewBranch(testCase.branch)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Merge(t *testing.T) {
	var testCases = []struct {
		name          string
		commitlike    string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedMerge bool
		expectedErr   bool
	}{
		{
			name:       "happy case",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
			},
			expectedMerge: true,
			expectedErr:   false,
		},
		{
			name:       "merge fails but abort succeeds",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
				{"merge", "--abort"},
			},
			expectedMerge: false,
			expectedErr:   false,
		},
		{
			name:       "merge fails and abort fails",
			commitlike: "shasum",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
				{"merge", "--abort"},
			},
			expectedMerge: false,
			expectedErr:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualMerge, actualErr := i.Merge(testCase.commitlike)
			if testCase.expectedMerge != actualMerge {
				t.Errorf("%s: got incorrect output: expected %v, got %v", testCase.name, testCase.expectedMerge, actualMerge)
			}
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_MergeWithStrategy(t *testing.T) {
	var testCases = []struct {
		name          string
		commitlike    string
		strategy      string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedMerge bool
		expectedErr   bool
	}{
		{
			name:       "happy merge case",
			commitlike: "shasum",
			strategy:   "merge",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
			},
			expectedMerge: true,
			expectedErr:   false,
		},
		{
			name:       "happy squash case",
			commitlike: "shasum",
			strategy:   "squash",
			responses: map[string]execResponse{
				"merge --squash --no-stat shasum": {
					out: []byte(`ok`),
				},
				"commit --no-stat -m merge": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--squash", "--no-stat", "shasum"},
				{"commit", "--no-stat", "-m", "merge"},
			},
			expectedMerge: true,
			expectedErr:   false,
		},
		{
			name:          "invalid strategy",
			commitlike:    "shasum",
			strategy:      "whatever",
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedMerge: false,
			expectedErr:   true,
		},
		{
			name:       "merge fails but abort succeeds",
			commitlike: "shasum",
			strategy:   "merge",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
				{"merge", "--abort"},
			},
			expectedMerge: false,
			expectedErr:   false,
		},
		{
			name:       "merge fails and abort fails",
			commitlike: "shasum",
			strategy:   "merge",
			responses: map[string]execResponse{
				"merge --no-ff --no-stat -m merge shasum": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "shasum"},
				{"merge", "--abort"},
			},
			expectedMerge: false,
			expectedErr:   true,
		},
		{
			name:       "squash merge fails but abort succeeds",
			commitlike: "shasum",
			strategy:   "squash",
			responses: map[string]execResponse{
				"merge --squash --no-stat shasum": {
					err: errors.New("oops"),
				},
				"reset --hard HEAD": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--squash", "--no-stat", "shasum"},
				{"reset", "--hard", "HEAD"},
			},
			expectedMerge: false,
			expectedErr:   false,
		},
		{
			name:       "squash merge fails and abort fails",
			commitlike: "shasum",
			strategy:   "squash",
			responses: map[string]execResponse{
				"merge --squash --no-stat shasum": {
					err: errors.New("oops"),
				},
				"reset --hard HEAD": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--squash", "--no-stat", "shasum"},
				{"reset", "--hard", "HEAD"},
			},
			expectedMerge: false,
			expectedErr:   true,
		},
		{
			name:       "squash merge staging succeeds, commit fails and abort succeeds",
			commitlike: "shasum",
			strategy:   "squash",
			responses: map[string]execResponse{
				"merge --squash --no-stat shasum": {
					out: []byte(`ok`),
				},
				"commit --no-stat -m merge": {
					err: errors.New("oops"),
				},
				"reset --hard HEAD": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--squash", "--no-stat", "shasum"},
				{"commit", "--no-stat", "-m", "merge"},
				{"reset", "--hard", "HEAD"},
			},
			expectedMerge: false,
			expectedErr:   false,
		},
		{
			name:       "squash merge staging succeeds, commit fails and abort fails",
			commitlike: "shasum",
			strategy:   "squash",
			responses: map[string]execResponse{
				"merge --squash --no-stat shasum": {
					out: []byte(`ok`),
				},
				"commit --no-stat -m merge": {
					err: errors.New("oops"),
				},
				"reset --hard HEAD": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"merge", "--squash", "--no-stat", "shasum"},
				{"commit", "--no-stat", "-m", "merge"},
				{"reset", "--hard", "HEAD"},
			},
			expectedMerge: false,
			expectedErr:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualMerge, actualErr := i.MergeWithStrategy(testCase.commitlike, testCase.strategy)
			if testCase.expectedMerge != actualMerge {
				t.Errorf("%s: got incorrect output: expected %v, got %v", testCase.name, testCase.expectedMerge, actualMerge)
			}
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_MergeAndCheckout(t *testing.T) {
	var testCases = []struct {
		name          string
		baseSHA       string
		commitlikes   []string
		strategy      string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:        "happy do nothing case",
			baseSHA:     "base",
			commitlikes: []string{},
			strategy:    "merge",
			responses: map[string]execResponse{
				"checkout base": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
			},
			expectedErr: false,
		},
		{
			name:        "happy merge case",
			baseSHA:     "base",
			commitlikes: []string{"first", "second"},
			strategy:    "merge",
			responses: map[string]execResponse{
				"checkout base": {
					out: []byte(`ok`),
				},
				"merge --no-ff --no-stat -m merge first": {
					out: []byte(`ok`),
				},
				"merge --no-ff --no-stat -m merge second": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "first"},
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "second"},
			},
			expectedErr: false,
		},
		{
			name:        "happy squash case",
			baseSHA:     "base",
			commitlikes: []string{"first", "second"},
			strategy:    "squash",
			responses: map[string]execResponse{
				"checkout base": {
					out: []byte(`ok`),
				},
				"merge --squash --no-stat first": {
					out: []byte(`ok`),
				},
				"commit --no-stat -m merge": {
					out: []byte(`ok`),
				},
				"merge --squash --no-stat second": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
				{"merge", "--squash", "--no-stat", "first"},
				{"commit", "--no-stat", "-m", "merge"},
				{"merge", "--squash", "--no-stat", "second"},
				{"commit", "--no-stat", "-m", "merge"},
			},
			expectedErr: false,
		},
		{
			name:          "invalid strategy",
			commitlikes:   []string{"shasum"},
			strategy:      "whatever",
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
		{
			name:        "checkout fails",
			baseSHA:     "base",
			commitlikes: []string{"first", "second"},
			strategy:    "squash",
			responses: map[string]execResponse{
				"checkout base": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
			},
			expectedErr: true,
		},
		{
			name:        "merge fails but abort succeeds",
			baseSHA:     "base",
			commitlikes: []string{"first", "second"},
			strategy:    "merge",
			responses: map[string]execResponse{
				"checkout base": {
					out: []byte(`ok`),
				},
				"merge --no-ff --no-stat -m merge first": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "first"},
				{"merge", "--abort"},
			},
			expectedErr: true,
		},
		{
			name:        "merge fails and abort fails",
			baseSHA:     "base",
			commitlikes: []string{"first", "second"},
			strategy:    "merge",
			responses: map[string]execResponse{
				"checkout base": {
					out: []byte(`ok`),
				},
				"merge --no-ff --no-stat -m merge first": {
					err: errors.New("oops"),
				},
				"merge --abort": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"checkout", "base"},
				{"merge", "--no-ff", "--no-stat", "-m", "merge", "first"},
				{"merge", "--abort"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.MergeAndCheckout(testCase.baseSHA, testCase.strategy, testCase.commitlikes...)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Am(t *testing.T) {
	var testCases = []struct {
		name          string
		path          string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			path: "my/changes.patch",
			responses: map[string]execResponse{
				"am --3way my/changes.patch": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"am", "--3way", "my/changes.patch"},
			},
			expectedErr: false,
		},
		{
			name: "am fails but abort succeeds",
			path: "my/changes.patch",
			responses: map[string]execResponse{
				"am --3way my/changes.patch": {
					err: errors.New("oops"),
				},
				"am --abort": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"am", "--3way", "my/changes.patch"},
				{"am", "--abort"},
			},
			expectedErr: true,
		},
		{
			name: "am fails and abort fails",
			path: "my/changes.patch",
			responses: map[string]execResponse{
				"am --3way my/changes.patch": {
					err: errors.New("oops"),
				},
				"am --abort": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"am", "--3way", "my/changes.patch"},
				{"am", "--abort"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.Am(testCase.path)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_RemoteUpdate(t *testing.T) {
	var testCases = []struct {
		name          string
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			responses: map[string]execResponse{
				"remote update": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"remote", "update"},
			},
			expectedErr: false,
		},
		{
			name: "update fails",
			responses: map[string]execResponse{
				"remote update": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"remote", "update"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.RemoteUpdate()
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Fetch(t *testing.T) {
	var testCases = []struct {
		name          string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com"},
			},
			expectedErr: false,
		},
		{
			name: "remote resolution fails",
			remote: func() (string, error) {
				return "", errors.New("oops")
			},
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
		{
			name: "fetch fails",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.Fetch()
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_FetchRef(t *testing.T) {
	var testCases = []struct {
		name          string
		refspec       string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:    "happy case",
			refspec: "shasum",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com shasum": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "shasum"},
			},
			expectedErr: false,
		},
		{
			name:    "remote resolution fails",
			refspec: "shasum",
			remote: func() (string, error) {
				return "", errors.New("oops")
			},
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
		{
			name:    "fetch fails",
			refspec: "shasum",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com shasum": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "shasum"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.FetchRef(testCase.refspec)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_FetchFromRemote(t *testing.T) {
	var testCases = []struct {
		name          string
		remote        RemoteResolver
		toRemote      RemoteResolver
		branch        string
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "fetch from different remote without token",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			toRemote: func() (string, error) {
				return "https://github.com/kubernetes/test-infra-fork", nil
			},
			branch: "test-branch",
			responses: map[string]execResponse{
				"fetch https://github.com/kubernetes/test-infra-fork test-branch": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "https://github.com/kubernetes/test-infra-fork", "test-branch"},
			},
			expectedErr: false,
		},
		{
			name: "fetch from different remote with token",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			toRemote: func() (string, error) {
				return "https://user:pass@github.com/kubernetes/test-infra-fork", nil
			},
			branch: "test-branch",
			responses: map[string]execResponse{
				"fetch https://user:pass@github.com/kubernetes/test-infra-fork test-branch": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "https://user:pass@github.com/kubernetes/test-infra-fork", "test-branch"},
			},
			expectedErr: false,
		},
		{
			name: "passing non-valid remote",
			remote: func() (string, error) {
				return "someone.com", nil
			},
			toRemote: func() (string, error) {
				return "", fmt.Errorf("non-valid URL")
			},
			branch:        "test-branch",
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}

			actualErr := i.FetchFromRemote(testCase.toRemote, testCase.branch)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_CheckoutPullRequest(t *testing.T) {
	var testCases = []struct {
		name          string
		number        int
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:   "happy case",
			number: 1,
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com pull/1/head": {
					out: []byte(`ok`),
				},
				"checkout FETCH_HEAD": {
					out: []byte(`ok`),
				},
				"checkout -b pull1": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "pull/1/head"},
				{"checkout", "FETCH_HEAD"},
				{"checkout", "-b", "pull1"},
			},
			expectedErr: false,
		},
		{
			name:   "remote resolution fails",
			number: 1,
			remote: func() (string, error) {
				return "", errors.New("oops")
			},
			responses:     map[string]execResponse{},
			expectedCalls: [][]string{},
			expectedErr:   true,
		},
		{
			name:   "fetch fails",
			number: 1,
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com pull/1/head": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "pull/1/head"},
			},
			expectedErr: true,
		},
		{
			name:   "checkout fails",
			number: 1,
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com pull/1/head": {
					out: []byte(`ok`),
				},
				"checkout FETCH_HEAD": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "pull/1/head"},
				{"checkout", "FETCH_HEAD"},
			},
			expectedErr: true,
		},
		{
			name:   "branch fails",
			number: 1,
			remote: func() (string, error) {
				return "someone.com", nil
			},
			responses: map[string]execResponse{
				"fetch someone.com pull/1/head": {
					out: []byte(`ok`),
				},
				"checkout FETCH_HEAD": {
					out: []byte(`ok`),
				},
				"checkout -b pull1": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"fetch", "someone.com", "pull/1/head"},
				{"checkout", "FETCH_HEAD"},
				{"checkout", "-b", "pull1"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.CheckoutPullRequest(testCase.number)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Config(t *testing.T) {
	var testCases = []struct {
		name          string
		key, value    string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name:  "happy case",
			key:   "key",
			value: "value",
			responses: map[string]execResponse{
				"config key value": {
					out: []byte(`ok`),
				},
			},
			expectedCalls: [][]string{
				{"config", "key", "value"},
			},
			expectedErr: false,
		},
		{
			name:  "config fails",
			key:   "key",
			value: "value",
			responses: map[string]execResponse{
				"config key value": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"config", "key", "value"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualErr := i.Config(testCase.key, testCase.value)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_Diff(t *testing.T) {
	var testCases = []struct {
		name          string
		head, sha     string
		remote        RemoteResolver
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedOut   []string
		expectedErr   bool
	}{
		{
			name: "happy case",
			head: "head",
			sha:  "sha",
			responses: map[string]execResponse{
				"diff head sha --name-only": {
					out: []byte(`prow/git/v2/client_factory.go
prow/git/v2/executor.go
prow/git/v2/executor_test.go
prow/git/v2/fakes.go
prow/git/v2/interactor.go
prow/git/v2/publisher.go
prow/git/v2/publisher_test.go
prow/git/v2/remote.go
prow/git/v2/remote_test.go`),
				},
			},
			expectedCalls: [][]string{
				{"diff", "head", "sha", "--name-only"},
			},
			expectedOut: []string{
				"prow/git/v2/client_factory.go",
				"prow/git/v2/executor.go",
				"prow/git/v2/executor_test.go",
				"prow/git/v2/fakes.go",
				"prow/git/v2/interactor.go",
				"prow/git/v2/publisher.go",
				"prow/git/v2/publisher_test.go",
				"prow/git/v2/remote.go",
				"prow/git/v2/remote_test.go",
			},
			expectedErr: false,
		},
		{
			name: "config fails",
			head: "head",
			sha:  "sha",
			responses: map[string]execResponse{
				"diff head sha --name-only": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"diff", "head", "sha", "--name-only"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				remote:   testCase.remote,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualOut, actualErr := i.Diff(testCase.head, testCase.sha)
			if !reflect.DeepEqual(actualOut, testCase.expectedOut) {
				t.Errorf("%s: got incorrect output: %v", testCase.name, diff.ObjectReflectDiff(actualOut, testCase.expectedOut))
			}
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_MergeCommitsExistBetween(t *testing.T) {
	var testCases = []struct {
		name          string
		target, head  string
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedOut   bool
		expectedErr   bool
	}{
		{
			name:   "happy case and merges exist",
			target: "target",
			head:   "head",
			responses: map[string]execResponse{
				"log target..head --oneline --merges": {
					out: []byte(`8df5654e6 Merge pull request #14911 from mborsz/etcd
96cbeee23 Merge pull request #14755 from justinsb/the_life_changing_magic_of_tidying_up`),
				},
			},
			expectedCalls: [][]string{
				{"log", "target..head", "--oneline", "--merges"},
			},
			expectedOut: true,
			expectedErr: false,
		},
		{
			name:   "happy case and merges don't exist",
			target: "target",
			head:   "head",
			responses: map[string]execResponse{
				"log target..head --oneline --merges": {
					out: []byte(``),
				},
			},
			expectedCalls: [][]string{
				{"log", "target..head", "--oneline", "--merges"},
			},
			expectedOut: false,
			expectedErr: false,
		},
		{
			name:   "log fails",
			target: "target",
			head:   "head",
			responses: map[string]execResponse{
				"log target..head --oneline --merges": {
					err: errors.New("oops"),
				},
			},
			expectedCalls: [][]string{
				{"log", "target..head", "--oneline", "--merges"},
			},
			expectedOut: false,
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				logger:   logrus.WithField("test", testCase.name),
			}
			actualOut, actualErr := i.MergeCommitsExistBetween(testCase.target, testCase.head)
			if testCase.expectedOut != actualOut {
				t.Errorf("%s: got incorrect output: expected %v, got %v", testCase.name, testCase.expectedOut, actualOut)
			}
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestInteractor_ShowRef(t *testing.T) {
	const target = "some-branch"
	var testCases = []struct {
		name          string
		responses     map[string]execResponse
		expectedCalls [][]string
		expectedErr   bool
	}{
		{
			name: "happy case",
			responses: map[string]execResponse{
				"show-ref -s some-branch": {out: []byte("32d3f5a6826109c625527f18a59f2e7144a330b6\n")},
			},
			expectedCalls: [][]string{
				{"show-ref", "-s", target},
			},
			expectedErr: false,
		},
		{
			name: "unhappy case",
			responses: map[string]execResponse{
				"git show-ref -s some-undef-branch": {err: errors.New("some-err")},
			},
			expectedCalls: [][]string{
				{"show-ref", "-s", target},
			},
			expectedErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := fakeExecutor{
				records:   [][]string{},
				responses: testCase.responses,
			}
			i := interactor{
				executor: &e,
				logger:   logrus.WithField("test", testCase.name),
			}
			_, actualErr := i.ShowRef(target)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual, expected := e.records, testCase.expectedCalls; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect git calls: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}
