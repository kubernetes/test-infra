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
	"bufio"
	"fmt"
	"os"
	"strings"

	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Print
)

// PathLabelMunger will add labels to PRs based on what files it modified.
// The mapping of files to labels if provided in a file in --path-label-config
type PathLabelMunger struct {
	labelMap      *map[string]string
	pathLabelFile string
}

func init() {
	RegisterMungerOrDie(&PathLabelMunger{})
}

// Name is the name usable in --pr-mungers
func (p *PathLabelMunger) Name() string { return "path-label" }

// Initialize will initialize the munger
func (p *PathLabelMunger) Initialize(config *github.Config) error {
	out := map[string]string{}
	p.labelMap = &out
	file := p.pathLabelFile
	if len(file) == 0 {
		glog.Infof("No --path-label-config= supplied, applying no labels")
		return nil
	}
	fp, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fp.Close()
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			glog.Errorf("Invalid line in path based label munger config %s: %q", file, line)
			continue
		}
		file := fields[0]
		label := fields[1]
		out[file] = label
	}
	return scanner.Err()
}

// EachLoop is called at the start of every munge loop
func (p *PathLabelMunger) EachLoop(_ *github.Config) error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (p *PathLabelMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&p.pathLabelFile, "path-label-config", "path-label.txt", "file containing the pathname to label mappings")
}

// Munge is the workhorse the will actually make updates to the PR
func (p *PathLabelMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	commits, err := obj.GetCommits()
	if err != nil {
		return
	}

	labelMap := *p.labelMap

	needsLabels := sets.NewString()
	for _, c := range commits {
		for _, f := range c.Files {
			for prefix, label := range labelMap {
				if strings.HasPrefix(*f.Filename, prefix) && !obj.HasLabel(label) {
					needsLabels.Insert(label)
				}
			}
		}
	}

	if needsLabels.Len() != 0 {
		obj.AddLabels(needsLabels.List())
	}
}
