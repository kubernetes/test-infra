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
	"os/exec"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/errors"
)

type info struct {
	repo string
	dir  string
}

// PublisherMunger publishes content from one repository to another one.
type PublisherMunger struct {
	// Command for the 'publisher' munger to run periodically.
	PublishCommand string
	// base to all repos
	baseDir string
	// location to write the netrc file needed for github authentication
	netrcDir     string
	srcToDst     map[info]info
	features     *features.Features
	githubConfig *github.Config
}

func init() {
	publisherMunger := &PublisherMunger{}
	RegisterMungerOrDie(publisherMunger)
}

// Name is the name usable in --pr-mungers
func (p *PublisherMunger) Name() string { return "publisher" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *PublisherMunger) RequiredFeatures() []string { return []string{features.RepoFeatureName} }

// Initialize will initialize the munger
func (p *PublisherMunger) Initialize(config *github.Config, features *features.Features) error {
	p.baseDir = features.Repos.BaseDir
	if len(p.baseDir) == 0 {
		glog.Fatalf("--repo-dir is required with selected munger(s)")
	}
	if len(p.PublishCommand) == 0 {
		glog.Fatalf("--publish-command is required with selected munger(s)")
	}
	p.srcToDst = make(map[info]info)
	p.srcToDst[info{repo: config.Project, dir: "staging/src/k8s.io/client-go"}] = info{repo: "client-go", dir: ""}
	glog.Infof("pulisher munger map: %#v\n", p.srcToDst)
	p.features = features
	p.githubConfig = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *PublisherMunger) EachLoop() error {
	var errlist []error
	for srcInfo, dstInfo := range p.srcToDst {
		src := filepath.Join(p.baseDir, srcInfo.repo, srcInfo.dir)
		srcURL := fmt.Sprintf("https://github.com/%s/%s.git", p.githubConfig.Org, srcInfo.repo)
		dst := filepath.Join(p.baseDir, dstInfo.repo, dstInfo.dir)
		dstURL := fmt.Sprintf("https://github.com/%s/%s.git", p.githubConfig.Org, dstInfo.repo)
		cmd := exec.Command(p.PublishCommand, src, dst, srcURL, dstURL, p.githubConfig.Token(), p.netrcDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			glog.Errorf("Failed to publish %s to %s.\nOutput: %s\nError: %s", src, dst, output, err)
			errlist = append(errlist, err)
		} else {
			glog.Infof("Successfully publish %s to %s: %s", src, dst, output)
		}
	}
	return errors.NewAggregate(errlist)
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *PublisherMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&p.PublishCommand, "publish-command", "", "Command for the 'publisher' munger to periodically run.")
	cmd.Flags().StringVar(&p.netrcDir, "netrc-dir", "", "Location to write the netrc file needed for github authentication.")
}

// Munge is the workhorse the will actually make updates to the PR
func (p *PublisherMunger) Munge(obj *github.MungeObject) {}
