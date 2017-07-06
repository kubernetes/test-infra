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
	"strings"

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

type branchRule struct {
	src coordinate
	dst coordinate
	// k8s.io/* repos the dst dependes on
	deps []coordinate
}

// a collection of publishing rules for a single destination repo
type repoRules struct {
	dstRepo string
	// publisher.go has assumption that src.repo is always kubernetes.
	srcToDst []branchRule
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
	k8sIOPath string
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
	p.k8sIOPath = filepath.Join(gopath, "src", "k8s.io")

	clientGo := repoRules{
		dstRepo: "client-go",
		srcToDst: []branchRule{
			{
				// rule for the client-go master branch
				src:  coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/client-go"},
				dst:  coordinate{repo: "client-go", branch: "master", dir: "./"},
				deps: []coordinate{coordinate{repo: "apimachinery", branch: "master"}},
			},
			{
				// rule for the client-go release-2.0 branch
				src: coordinate{repo: config.Project, branch: "release-1.5", dir: "staging/src/k8s.io/client-go"},
				dst: coordinate{repo: "client-go", branch: "release-2.0", dir: "./"},
			},
			{
				// rule for the client-go release-3.0 branch
				src:  coordinate{repo: config.Project, branch: "release-1.6", dir: "staging/src/k8s.io/client-go"},
				dst:  coordinate{repo: "client-go", branch: "release-3.0", dir: "./"},
				deps: []coordinate{coordinate{repo: "apimachinery", branch: "release-1.6"}},
			},
			{
				// rule for the client-go release-4.0 branch
				src:  coordinate{repo: config.Project, branch: "release-1.7", dir: "staging/src/k8s.io/client-go"},
				dst:  coordinate{repo: "client-go", branch: "release-4.0", dir: "./"},
				deps: []coordinate{coordinate{repo: "apimachinery", branch: "release-1.7"}},
			},
		},
		publishScript: "/publish_scripts/publish_client_go.sh",
	}

	apimachinery := repoRules{
		dstRepo: "apimachinery",
		srcToDst: []branchRule{
			{
				// rule for the apimachinery master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "master", dir: "./"},
			},
			{
				// rule for the apimachinery 1.6 branch
				src: coordinate{repo: config.Project, branch: "release-1.6", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.6", dir: "./"},
			},
			{
				// rule for the apimachinery 1.7 branch
				src: coordinate{repo: config.Project, branch: "release-1.7", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.7", dir: "./"},
			},
		},
		publishScript: "/publish_scripts/publish_apimachinery.sh",
	}

	apiserver := repoRules{
		dstRepo: "apiserver",
		srcToDst: []branchRule{
			{
				// rule for the apiserver master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "master"},
					coordinate{repo: "client-go", branch: "master"},
				},
			},
			{
				// rule for the apiserver 1.6 branch
				src: coordinate{repo: config.Project, branch: "release-1.6", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "release-1.6"},
					coordinate{repo: "client-go", branch: "release-3.0"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_apiserver.sh",
	}

	kubeAggregator := repoRules{
		dstRepo: "kube-aggregator",
		srcToDst: []branchRule{
			{
				// rule for the kube-aggregator master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "master", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "master"},
					coordinate{repo: "client-go", branch: "master"},
					coordinate{repo: "apiserver", branch: "master"},
				},
			},
			{
				// rule for the kube-aggregator 1.6 branch
				src: coordinate{repo: config.Project, branch: "release-1.6", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "release-1.6"},
					coordinate{repo: "client-go", branch: "release-3.0"},
					coordinate{repo: "apiserver", branch: "release-1.6"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_kube_aggregator.sh",
	}

	sampleAPIServer := repoRules{
		dstRepo: "sample-apiserver",
		srcToDst: []branchRule{
			{
				// rule for the sample-apiserver master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "master"},
					coordinate{repo: "client-go", branch: "master"},
					coordinate{repo: "apiserver", branch: "master"},
				},
			},
			{
				// rule for the sample-apiserver 1.6 branch
				src: coordinate{repo: config.Project, branch: "release-1.6", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "release-1.6"},
					coordinate{repo: "client-go", branch: "release-3.0"},
					coordinate{repo: "apiserver", branch: "release-1.6"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_sample_apiserver.sh",
	}

	apiExtensionsAPIServer := repoRules{
		dstRepo: "apiextensions-apiserver",
		srcToDst: []branchRule{
			{
				// rule for the sample-apiserver master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/apiextensions-apiserver"},
				dst: coordinate{repo: "apiextensions-apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "master"},
					coordinate{repo: "client-go", branch: "master"},
					coordinate{repo: "apiserver", branch: "master"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_apiextensions_apiserver.sh",
	}

	api := repoRules{
		dstRepo: "api",
		srcToDst: []branchRule{
			{
				// rule for the api master branch
				src: coordinate{repo: config.Project, branch: "master", dir: "staging/src/k8s.io/api"},
				dst: coordinate{repo: "api", branch: "master", dir: "./"},
				deps: []coordinate{
					coordinate{repo: "apimachinery", branch: "master"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_api.sh",
	}
	// NOTE: Order of the repos is sensitive!!! A dependent repo needs to be published first, so that other repos can vendor its latest revision.
	p.reposRules = []repoRules{apimachinery, api, clientGo, apiserver, kubeAggregator, sampleAPIServer, apiExtensionsAPIServer}
	glog.Infof("publisher munger rules: %#v\n", p.reposRules)
	p.features = features
	p.githubConfig = config
	return nil
}

// update the local checkout of k8s.io/kubernetes
func (p *PublisherMunger) updateKubernetes() error {
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
	output, err := cmd.CombinedOutput()
	p.plog.Infof("%s", output)
	if err != nil {
		return err
	}
	// update kubernetes branches that are needed by other k8s.io repos.
	for _, repoRules := range p.reposRules {
		for _, branchRule := range repoRules.srcToDst {
			src := branchRule.src
			// we assume src.repo is always kubernetes
			cmd := exec.Command("git", "branch", "-f", src.branch, fmt.Sprintf("origin/%s", src.branch))
			cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
			output, err := cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err == nil {
				continue
			}
			// probably the error is because we cannot do `git branch -f` while
			// current branch is src.branch, so try `git reset --hard` instead.
			cmd = exec.Command("git", "reset", "--hard", fmt.Sprintf("origin/%s", src.branch))
			cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
			output, err = cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// git clone dstURL to dst if dst doesn't exist yet.
func (p *PublisherMunger) ensureCloned(dst string, dstURL string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	err := exec.Command("mkdir", "-p", dst).Run()
	if err != nil {
		return err
	}
	err = exec.Command("git", "clone", dstURL, dst).Run()
	return err
}

// constructs all the repos, but does not push the changes to remotes.
func (p *PublisherMunger) construct() error {
	kubernetesRemote := filepath.Join(p.k8sIOPath, "kubernetes", ".git")
	for _, repoRules := range p.reposRules {
		// clone the destination repo
		dstDir := filepath.Join(p.k8sIOPath, repoRules.dstRepo, "")
		dstURL := fmt.Sprintf("https://github.com/%s/%s.git", p.githubConfig.Org, repoRules.dstRepo)
		if err := p.ensureCloned(dstDir, dstURL); err != nil {
			p.plog.Errorf("%v", err)
			return err
		}
		p.plog.Infof("Successfully ensured %s exists", dstDir)
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		// construct branches
		formatDeps := func(deps []coordinate) string {
			var depStrings []string
			for _, dep := range deps {
				depStrings = append(depStrings, fmt.Sprintf("%s:%s", dep.repo, dep.branch))
			}
			return strings.Join(depStrings, ",")
		}

		for _, branchRule := range repoRules.srcToDst {
			cmd := exec.Command(repoRules.publishScript, branchRule.src.branch, branchRule.dst.branch, formatDeps(branchRule.deps), kubernetesRemote)
			output, err := cmd.CombinedOutput()
			p.plog.Infof("%s", output)
			if err != nil {
				return err
			}
			p.plog.Infof("Successfully constructed %s", branchRule.dst)
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
		dstDir := filepath.Join(p.k8sIOPath, repoRules.dstRepo, "")
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		for _, branchRule := range repoRules.srcToDst {
			cmd := exec.Command("/publish_scripts/push.sh", p.githubConfig.Token(), branchRule.dst.branch)
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

	if err := p.updateKubernetes(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	if err := p.construct(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	if err := p.publish(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *PublisherMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (p *PublisherMunger) Munge(obj *github.MungeObject) {}
