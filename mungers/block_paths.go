/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"os"
	"regexp"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/yaml"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	blockPathFormat = `Adding label:%s because PR changes docs prohibited to auto merge
See http://kubernetes.io/editdocs/ for information about editing docs`
)

var (
	_             = fmt.Print
	blockPathBody = fmt.Sprintf(blockPathFormat, doNotMergeLabel)
)

type configBlockPath struct {
	BlockRegexp      []string `json:"blockRegexp,omitempty" yaml:"blockRegexp,omitempty"`
	DoNotBlockRegexp []string `json:"doNotBlockRegexp,omitempty" yaml:"doNotBlockRegexp,omitempty"`
}

// BlockPath will add a label to block auto merge if a PR touches certain paths
type BlockPath struct {
	Path             string
	blockRegexp      []regexp.Regexp
	doNotBlockRegexp []regexp.Regexp
}

func init() {
	b := &BlockPath{}
	RegisterMungerOrDie(b)
	RegisterStaleComments(b)
}

// Name is the name usable in --pr-mungers
func (b *BlockPath) Name() string { return "block-path" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *BlockPath) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (b *BlockPath) Initialize(config *github.Config, features *features.Features) error {
	if len(b.Path) == 0 {
		glog.Fatalf("--block-path-config is required with the block-path munger")
	}
	file, err := os.Open(b.Path)
	if err != nil {
		glog.Fatalf("Failed to load block-path config: %v", err)
	}
	defer file.Close()

	c := &configBlockPath{}
	if err := yaml.NewYAMLToJSONDecoder(file).Decode(c); err != nil {
		glog.Fatalf("Failed to decode the block-path config: %v", err)
	}

	b.blockRegexp = []regexp.Regexp{}
	for _, str := range c.BlockRegexp {
		reg, err := regexp.Compile(str)
		if err != nil {
			return err
		}
		b.blockRegexp = append(b.blockRegexp, *reg)
	}

	b.doNotBlockRegexp = []regexp.Regexp{}
	for _, str := range c.DoNotBlockRegexp {
		reg, err := regexp.Compile(str)
		if err != nil {
			return err
		}
		b.doNotBlockRegexp = append(b.doNotBlockRegexp, *reg)
	}
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *BlockPath) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (b *BlockPath) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&b.Path, "block-path-config", "", "file containing the pathnames to block or not block")
}

func matchesAny(path string, regs []regexp.Regexp) bool {
	for _, reg := range regs {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

// Munge is the workhorse the will actually make updates to the PR
func (b *BlockPath) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if obj.HasLabel(doNotMergeLabel) {
		return
	}

	files, err := obj.ListFiles()
	if err != nil {
		return
	}

	for _, f := range files {
		if matchesAny(*f.Filename, b.blockRegexp) {
			if matchesAny(*f.Filename, b.doNotBlockRegexp) {
				continue
			}
			obj.WriteComment(blockPathBody)
			obj.AddLabels([]string{doNotMergeLabel})
			return
		}
	}
}

func (b *BlockPath) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != blockPathBody {
		return false
	}
	stale := !obj.HasLabel(doNotMergeLabel)
	if stale {
		glog.V(6).Infof("Found stale BlockPath comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (b *BlockPath) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, b.isStaleComment)
}
