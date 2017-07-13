/*
Copyright 2015 The Kubernetes Authors.

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
	"path"
	"regexp"

	githubapi "github.com/google/go-github/github"
	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"
)

const (
	labelSkipRetest = "retest-not-required-docs-only"
)

var (
	docFilesRegex    = regexp.MustCompile("^.*\\.(md|png|svg|dia)$")
	ownersFilesRegex = regexp.MustCompile("^OWNERS$")
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

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (DocsNeedNoRetest) RegisterOptions(opts *options.Options) sets.String { return nil }

func areFilesDocOnly(files []*githubapi.CommitFile) bool {
	for _, file := range files {
		_, basename := path.Split(*file.Filename)
		if docFilesRegex.MatchString(basename) {
			continue
		}
		if ownersFilesRegex.MatchString(basename) {
			continue
		}
		return false
	}
	return true
}

// Munge is the workhorse the will actually make updates to the PR
func (DocsNeedNoRetest) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	docsOnly := areFilesDocOnly(files)
	if docsOnly && !obj.HasLabel(labelSkipRetest) {
		obj.AddLabel(labelSkipRetest)
	} else if !docsOnly && obj.HasLabel(labelSkipRetest) {
		obj.RemoveLabel(labelSkipRetest)
	}
}
