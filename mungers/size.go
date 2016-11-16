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
	"fmt"
	"path"
	"regexp"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	labelSizePrefix = "size/"
)

var (
	sizeRE = regexp.MustCompile("Labelling this PR as " + labelSizePrefix + "(XS|S|M|L|XL|XXL)")
)

// SizeMunger will update a label on a PR based on how many lines are changed.
// It will exclude certain files in it's calculations based on the config
// file provided in --generated-files-config
type SizeMunger struct {
	GeneratedFilesFile string
	genFilePaths       sets.String
	genFilePrefixes    sets.String
	genFileNames       sets.String
	genPathPrefixes    sets.String
}

func init() {
	s := &SizeMunger{}
	RegisterMungerOrDie(s)
	RegisterStaleComments(s)
}

// Name is the name usable in --pr-mungers
func (SizeMunger) Name() string { return "size" }

// RequiredFeatures is a slice of 'features' that must be provided
func (SizeMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (s *SizeMunger) Initialize(config *github.Config, features *features.Features) error {
	glog.Infof("generated-files-config: %#v\n", s.GeneratedFilesFile)

	return nil
}

// EachLoop is called at the start of every munge loop
func (SizeMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (s *SizeMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&s.GeneratedFilesFile, "generated-files-config", "", "file in the repo containing the generated file rules")
}

// getGeneratedFiles returns a list of all automatically generated files in the repo. These include
// docs, deep_copy, and conversions
//
// It would be 'better' to call this for every commit but that takes
// a whole lot of time for almost always the same information, and if
// our results are slightly wrong, who cares? Instead look for the
// generated files once and if someone changed what files are generated
// we'll size slightly wrong. No biggie.
func (s *SizeMunger) getGeneratedFiles(obj *github.MungeObject) {
	if s.genFilePaths != nil {
		return
	}
	if s.genFilePrefixes != nil {
		return
	}
	if s.genFileNames != nil {
		return
	}
	if s.genPathPrefixes != nil {
		return
	}

	// Don't write into s yet, since this might fail, and these are indicators
	// to not repeat this function.
	paths := sets.NewString()
	filePrefixes := sets.NewString()
	fileNames := sets.NewString()
	pathPrefixes := sets.NewString()

	file := s.GeneratedFilesFile
	if len(file) == 0 {
		glog.Infof("No --generated-files-config= supplied, applying no labels")
		return
	}

	contents, err := obj.GetFileContents(file, "")
	if err != nil {
		glog.Errorf("Failed to read generated-files-config %q: %v", file, err)
	}

	lines := strings.Split(contents, "\n")
	for i, line := range lines {
		line := strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			glog.Errorf("Invalid line %d in generated docs config %s: %q", i, file, line)
			continue
		}
		key := fields[0]
		val := fields[1]
		if key == "prefix" || key == "path-prefix" {
			pathPrefixes.Insert(val)
		} else if key == "file-prefix" {
			filePrefixes.Insert(val)
		} else if key == "file-name" {
			fileNames.Insert(val)
		} else if key == "path" {
			paths.Insert(val)
		} else if key == "paths-from-repo" {
			docs, err := obj.GetFileContents(val, "")
			if err != nil {
				continue
			}
			docSlice := strings.Split(docs, "\n")
			paths.Insert(docSlice...)
		} else {
			glog.Errorf("Invalid line %d in generated docs config, unknown key: %s, %q", i, key, line)
			continue
		}
	}

	// Save the results so we don't repeat this function.
	s.genFilePaths = paths
	s.genFilePrefixes = filePrefixes
	s.genFileNames = fileNames
	s.genPathPrefixes = pathPrefixes

	return
}

// Munge is the workhorse the will actually make updates to the PR
func (s *SizeMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	issue := obj.Issue

	s.getGeneratedFiles(obj)

	files, err := obj.ListFiles()
	if err != nil {
		return
	}

	adds := 0
	dels := 0
	for _, f := range files {
		skip := false
		for p := range s.genPathPrefixes {
			if strings.HasPrefix(*f.Filename, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if s.genFilePaths.Has(*f.Filename) {
			continue
		}
		_, filename := path.Split(*f.Filename)
		if hasPrefix(filename, s.genFilePrefixes) {
			continue
		}
		if s.genFileNames.Has(filename) {
			continue
		}
		if f.Additions != nil {
			adds += *f.Additions
		}
		if f.Deletions != nil {
			dels += *f.Deletions
		}
	}

	newSize := calculateSize(adds, dels)
	newLabel := labelSizePrefix + newSize

	existing := github.GetLabelsWithPrefix(issue.Labels, labelSizePrefix)
	needsUpdate := true
	for _, l := range existing {
		if l == newLabel {
			needsUpdate = false
			continue
		}
		obj.RemoveLabel(l)
	}
	if needsUpdate {
		obj.AddLabels([]string{newLabel})

		body := fmt.Sprintf("Labelling this PR as %s", newLabel)
		obj.WriteComment(body)
	}
}

func hasPrefix(filename string, filePrefixes sets.String) bool {
	for pfx := range filePrefixes {
		if strings.HasPrefix(filename, pfx) {
			return true
		}
	}
	return false
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

func (s *SizeMunger) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	stale := sizeRE.MatchString(*comment.Body)
	if stale {
		glog.V(6).Infof("Found stale SizeMunger comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (s *SizeMunger) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, s.isStaleComment)
}
