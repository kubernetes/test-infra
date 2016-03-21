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
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Print
)

type configBlockPath struct {
	BlockRegexp      []string `json:"blockRegexp,omitempty" yaml:"blockRegexp,omitempty"`
	DoNotBlockRegexp []string `json:"doNotBlockRegexp,omitempty" yaml:"doNotBlockRegexp,omitempty"`
}

// BlockPath will add a label to block auto merge if a PR touches certain paths
type BlockPath struct {
	path             string
	blockRegexp      []regexp.Regexp
	doNotBlockRegexp []regexp.Regexp
}

func init() {
	RegisterMungerOrDie(&BlockPath{})
}

// Name is the name usable in --pr-mungers
func (b *BlockPath) Name() string { return "block-path" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *BlockPath) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (b *BlockPath) Initialize(config *github.Config, features *features.Features) error {
	if len(b.path) == 0 {
		glog.Fatalf("--block-path-config is required with the block-path munger")
	}
	file, err := os.Open(b.path)
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
	cmd.Flags().StringVar(&b.path, "block-path-config", "block-path.yaml", "file containing the pathnames to block or not block")
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

	commits, err := obj.GetCommits()
	if err != nil {
		return
	}

	for _, c := range commits {
		for _, f := range c.Files {
			if matchesAny(*f.Filename, b.blockRegexp) {
				if matchesAny(*f.Filename, b.doNotBlockRegexp) {
					continue
				}
				body := fmt.Sprintf(`Adding label:%s because PR changes docs prohibited to auto merge
See http://kubernetes.io/editdocs/ for information about editing docs`, doNotMergeLabel)
				obj.WriteComment(body)
				obj.AddLabels([]string{doNotMergeLabel})
				return
			}
		}
	}
}
