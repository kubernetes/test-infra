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

package pulls

import (
	"fmt"
	"strings"

	github_util "k8s.io/contrib/github"
	"k8s.io/contrib/mungegithub/config"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

type PRSizeMunger struct {
	genFiles *sets.String
}

func init() {
	RegisterMungerOrDie(&PRSizeMunger{})
}

func (PRSizeMunger) Name() string { return "size" }

func (PRSizeMunger) AddFlags(cmd *cobra.Command) {}

const labelSizePrefix = "size/"

// getGeneratedFiles returns a list of all automatically generated files in the repo. These include
// docs, deep_copy, and conversions
//
// It would be 'better' to call this for every commit but that takes
// a whole lot of time for almost always the same information, and if
// our results are slightly wrong, who cares? Instead look for the
// generated files once and if someone changed what files are generated
// we'll size slightly wrong. No biggie.
func (s *PRSizeMunger) getGeneratedFiles(config *config.MungeConfig) sets.String {
	if s.genFiles != nil {
		return *s.genFiles
	}
	files := sets.NewString()
	files.Insert("pkg/api/v1/deep_copy_generated.go")
	files.Insert("pkg/api/deep_copy_generated.go")
	files.Insert("pkg/expapi/v1/deep_copy_generated.go")
	files.Insert("pkg/expapi/deep_copy_generated.go")
	files.Insert("pkg/api/v1/conversion_generated.go")
	files.Insert("pkg/expapi/v1/conversion_generated.go")
	files.Insert("api/swagger-spec/resourceListing.json")
	files.Insert("api/swagger-spec/version.json")
	files.Insert("api/swagger-spec/api.json")
	files.Insert("api/swagger-spec/v1.json")
	docs, err := config.GetFileContents(".generated_docs", "")
	if err != nil {
		docs = ""
	}
	docSlice := strings.Split(docs, "\n")
	files.Insert(docSlice...)

	s.genFiles = &files
	return *s.genFiles
}

func (s *PRSizeMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	genFiles := s.getGeneratedFiles(config)

	if pr.Additions == nil {
		glog.Warningf("PR %d has nil Additions", *pr.Number)
		return
	}
	adds := *pr.Additions
	if pr.Deletions == nil {
		glog.Warningf("PR %d has nil Deletions", *pr.Number)
		return
	}
	dels := *pr.Deletions

	for _, c := range commits {
		for _, f := range c.Files {
			if strings.HasPrefix(*f.Filename, "Godeps/") {
				adds = adds - *f.Additions
				dels = dels - *f.Deletions
				continue
			}
			if genFiles.Has(*f.Filename) {
				adds = adds - *f.Additions
				dels = dels - *f.Deletions
				continue
			}
		}
	}

	newSize := calculateSize(adds, dels)
	newLabel := labelSizePrefix + newSize

	existing := github_util.GetLabelsWithPrefix(issue.Labels, labelSizePrefix)
	needsUpdate := true
	for _, l := range existing {
		if l == newLabel {
			needsUpdate = false
			continue
		}
		config.RemoveLabel(*pr.Number, l)
	}
	if needsUpdate {
		config.AddLabels(*pr.Number, []string{newLabel})

		body := fmt.Sprintf("Labelling this PR as %s", newLabel)
		config.WriteComment(*pr.Number, body)
	}
}

const (
	sizeXS  = "XS"
	sizeS   = "S"
	sizeM   = "M"
	sizeL   = "L"
	sizeXL  = "XL"
	sizeXXL = "XXL"
)

func calculateSize(adds, dels int) string {
	lines := adds + dels

	// This is a totally arbitrary heuristic and is open for tweaking.
	if lines < 10 {
		return sizeXS
	}
	if lines < 30 {
		return sizeS
	}
	if lines < 100 {
		return sizeM
	}
	if lines < 500 {
		return sizeL
	}
	if lines < 1000 {
		return sizeXL
	}
	return sizeXXL
}
