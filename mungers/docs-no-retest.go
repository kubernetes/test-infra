/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"regexp"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
)

const (
	labelSkipRetest = "retest-not-required-docs-only"
)

var (
	ignoreFilesRegex = regexp.MustCompile(".*\\.md$")
)

// DocsNeedNoRetest automatically labels documentation only pull-requests as retest-not-required
type DocsNeedNoRetest struct{}

func init() {
	munger := &DocsNeedNoRetest{}
	RegisterMungerOrDie(munger)
}

// Name is the name usable in --pr-mungers
func (DocsNeedNoRetest) Name() string { return "docs-need-no-retest" }

// RequiredFeatures is a slice of 'features' that must be provided
func (DocsNeedNoRetest) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (s *DocsNeedNoRetest) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (DocsNeedNoRetest) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (DocsNeedNoRetest) AddFlags(cmd *cobra.Command, config *github.Config) {
}

func areFilesDocOnly(files []*githubapi.CommitFile) bool {
	for _, file := range files {
		if !ignoreFilesRegex.MatchString(*file.Filename) {
			return false
		}
	}

	return true
}

// Munge is the workhorse the will actually make updates to the PR
func (DocsNeedNoRetest) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	files, err := obj.ListFiles()
	if err != nil {
		glog.Errorf("Failed to list files for PR %d: %s", obj.Issue.Number, err)
	}

	docsOnly := areFilesDocOnly(files)
	if docsOnly && !obj.HasLabel(labelSkipRetest) {
		obj.AddLabel(labelSkipRetest)
	} else if !docsOnly && obj.HasLabel(labelSkipRetest) {
		obj.RemoveLabel(labelSkipRetest)
	}
}
