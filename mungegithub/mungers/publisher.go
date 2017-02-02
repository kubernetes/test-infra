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

package mungers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
)

// coordinate of a piece of code
type coordinate struct {
	repo   string
	branch string
	// dir from repo root
	dir string
}

func (c coordinate) String() string {
	return fmt.Sprintf("[repository %s, branch %s, subdir %s]", c.repo, c.branch, c.dir)
}

// a collection of publishing rules for a single destination repo
type repoRules struct {
	dstRepo  string
	srcToDst map[coordinate]coordinate
	// if empty (e.g., for client-go), publisher will use its default publish script
	publishScript string
}

// PublisherMunger publishes content from one repository to another one.
type PublisherMunger struct {
	// Command for the 'publisher' munger to run periodically.
	PublishCommand string
	// location to write the netrc file needed for github authentication
	netrcDir string
	// location of the plog output
	logDir       string
	reposRules   []repoRules
	features     *features.Features
	githubConfig *github.Config
	// plog duplicates the logs at glog and a file
	plog *plog
	// absolute path to the k8s repos.
	kubernetesPath string
}

func init() {
	publisherMunger := &PublisherMunger{}
	RegisterMungerOrDie(publisherMunger)
}

// Name is the name usable in --pr-mungers
func (p *PublisherMunger) Name() string { return "publisher" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *PublisherMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (p *PublisherMunger) Initialize(config *github.Config, features *features.Features) error {
	gopath := os.Getenv("GOPATH")
	p.kubernetesPath = filepath.Join(gopath, "src", "k8s.io")

	clientGo := repoRules{
		dstRepo: "client-go",
		srcToDst: map[coordinate]coordinate{
			// rule for the client-go master branch
			coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/client-go"}: coordinate{repo: "client-go", branch: "master", dir: "./"},
			// rule for the client-go release-2.0 branch
			coordinate{repo: config.Project, branch: "release-1.5", dir: "staging/src/k8s.io/client-go"}: coordinate{repo: "client-go", branch: "release-2.0", dir: "./"},
		},
		publishScript: "/publish_scripts/publish_client_go.sh",
	}

	apimachinery := repoRules{
		dstRepo: "apimachinery",
		srcToDst: map[coordinate]coordinate{
			// rule for the apimachinery master branch
			coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/apimachinery"}: coordinate{repo: "apimachinery", branch: "master", dir: "./"},
		},
		publishScript: "/publish_scripts/publish_apimachinery.sh",
	}

	// Order of the repos is sensitive. A dependent repo needs to be published first, so that other repos vendor the latest dependent repo.
	p.reposRules = []repoRules{apimachinery, clientGo}
	glog.Infof("publisher munger rules: %#v\n", p.reposRules)
	p.features = features
	p.githubConfig = config
	return nil
}

// git clone dstURL to dst
func clone(dst string, dstURL string) error {
	err := exec.Command("rm", "-rf", dst).Run()
	if err != nil {
		return err
	}
	err = exec.Command("mkdir", "-p", dst).Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "clone", dstURL, dst).Run()
	if err != nil {
		return err
	}
	return nil
}

// constructs all the repos, but does not push the changes to remotes.
func (p *PublisherMunger) construct() error {
	for _, repoRules := range p.reposRules {
		// clone the destination repo
		dstDir := filepath.Join(p.kubernetesPath, repoRules.dstRepo, "")
		dstURL := fmt.Sprintf("https://github.com/%s/%s.git", p.githubConfig.Org, repoRules.dstRepo)
		if err := clone(dstDir, dstURL); err != nil {
			return err
		}
		p.plog.Infof("Successfully cloned %s", dstURL)
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		// construct branches
		for src, dst := range repoRules.srcToDst {
			cmd := exec.Command("git", "checkout", dst.branch)
			output, err := cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err != nil {
				return err
			}

			cmd = exec.Command(repoRules.publishScript, src.branch, dst.branch)
			output, err = cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err != nil {
				return err
			}
			p.plog.Infof("Successfully constructed %s", dst)
		}
	}
	return nil
}

// publish to remotes.
func (p *PublisherMunger) publish() error {
	// NOTE: because some repos depend on each other, e.g., client-go depends on
	// apimachinery, they should be published atomically, but it's not supported
	// by github.
	for _, repoRules := range p.reposRules {
		dstDir := filepath.Join(p.kubernetesPath, repoRules.dstRepo, "")
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		for _, dst := range repoRules.srcToDst {
			cmd := exec.Command("/publish_scripts/push.sh", p.githubConfig.Token(), dst.branch)
			output, err := cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *PublisherMunger) EachLoop() error {
	buf := bytes.NewBuffer(nil)
	p.plog = NewPublisherLog(buf)

	if err := p.construct(); err != nil {
		p.plog.Errorf("%v", err)
		return err
	}
	if err := p.publish(); err != nil {
		p.plog.Errorf("%v", err)
		return err
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *PublisherMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (p *PublisherMunger) Munge(obj *github.MungeObject) {}
