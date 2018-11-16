/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/logrusutil"

	"github.com/sirupsen/logrus"
)

type options struct {
	config    string
	jobConfig string
	confirm   bool
	github    flagutil.GitHubOptions
}

func (o *options) Validate() error {
	if err := o.github.Validate(!o.confirm); err != nil {
		return err
	}

	if o.config == "" {
		return errors.New("empty --config-path")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.config, "config-path", "", "Path to prow config.yaml")
	fs.StringVar(&o.jobConfig, "job-config-path", "", "Path to prow job configs.")
	fs.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	o.github.AddFlags(fs)
	fs.Parse(os.Args[1:])
	return o
}

type requirements struct {
	Org     string
	Repo    string
	Branch  string
	Request *github.BranchProtectionRequest
}

// Errors holds a list of errors, including a method to concurrently append.
type Errors struct {
	lock sync.Mutex
	errs []error
}

func (e *Errors) add(err error) {
	e.lock.Lock()
	logrus.Info(err)
	defer e.lock.Unlock()
	e.errs = append(e.errs, err)
}

func main() {
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "branchprotector"}),
	)

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatal(err)
	}

	cfg, err := config.Load(o.config, o.jobConfig)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to load --config-path=%s", o.config)
	}

	secretAgent := &config.SecretAgent{}
	if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, !o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	githubClient.Throttle(300, 100) // 300 hourly tokens, bursts of 100

	p := protector{
		client:         githubClient,
		cfg:            cfg,
		updates:        make(chan requirements),
		errors:         Errors{},
		completedRepos: make(map[string]bool),
		done:           make(chan []error),
	}

	go p.configureBranches()
	p.protect()
	close(p.updates)
	errors := <-p.done
	if n := len(errors); n > 0 {
		for i, err := range errors {
			logrus.WithError(err).Error(i)
		}
		logrus.Fatalf("Encountered %d errors protecting branches", n)
	}
}

type client interface {
	RemoveBranchProtection(org, repo, branch string) error
	UpdateBranchProtection(org, repo, branch string, config github.BranchProtectionRequest) error
	GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error)
	GetRepos(org string, user bool) ([]github.Repo, error)
}

type protector struct {
	client         client
	cfg            *config.Config
	updates        chan requirements
	errors         Errors
	completedRepos map[string]bool
	done           chan []error
}

func (p *protector) configureBranches() {
	for u := range p.updates {
		if u.Request == nil {
			if err := p.client.RemoveBranchProtection(u.Org, u.Repo, u.Branch); err != nil {
				p.errors.add(fmt.Errorf("remove %s/%s=%s protection failed: %v", u.Org, u.Repo, u.Branch, err))
			}
			continue
		}

		if err := p.client.UpdateBranchProtection(u.Org, u.Repo, u.Branch, *u.Request); err != nil {
			p.errors.add(fmt.Errorf("update %s/%s=%s protection to %v failed: %v", u.Org, u.Repo, u.Branch, *u.Request, err))
		}
	}
	p.done <- p.errors.errs
}

// protect protects branches specified in the presubmit and branch-protection config sections.
func (p *protector) protect() {
	bp := p.cfg.BranchProtection

	// Scan the branch-protection configuration
	for orgName := range bp.Orgs {
		if org, err := bp.GetOrg(orgName); err != nil {
			p.errors.add(fmt.Errorf("get %s: %v", orgName, err))
		} else if err = p.UpdateOrg(orgName, *org); err != nil {
			p.errors.add(fmt.Errorf("update %s: %v", orgName, err))
		}
	}

	// Do not automatically protect tested repositories
	if !bp.ProtectTested {
		return
	}

	// Some repos with presubmits might not be listed in the branch-protection
	for repo := range p.cfg.Presubmits {
		if p.completedRepos[repo] == true {
			continue
		}
		parts := strings.Split(repo, "/")
		if len(parts) != 2 { // TODO(fejta): use a strong type here instead
			p.errors.add(fmt.Errorf("bad presubmit repo: %s", repo))
			continue
		}
		orgName := parts[0]
		repoName := parts[1]
		if org, err := bp.GetOrg(orgName); err != nil {
			p.errors.add(fmt.Errorf("get %s: %v", orgName, err))
		} else if repo, err := org.GetRepo(repoName); err != nil {
			p.errors.add(fmt.Errorf("get %s/%s: %v", orgName, repoName, err))
		} else if err = p.UpdateRepo(orgName, repoName, *repo); err != nil {
			p.errors.add(fmt.Errorf("update %s/%s: %v", orgName, repoName, err))
		}
	}
}

// UpdateOrg updates all repos in the org with the specified defaults
func (p *protector) UpdateOrg(orgName string, org config.Org) error {
	var repos []string
	if org.Protect != nil {
		// Strongly opinionated org, configure every repo in the org.
		rs, err := p.client.GetRepos(orgName, false)
		if err != nil {
			return fmt.Errorf("list repos: %v", err)
		}
		for _, r := range rs {
			if !r.Archived {
				repos = append(repos, r.Name)
			}
		}
	} else {
		// Unopinionated org, just set explicitly defined repos
		for r := range org.Repos {
			repos = append(repos, r)
		}
	}

	for _, repoName := range repos {
		if repo, err := org.GetRepo(repoName); err != nil {
			return fmt.Errorf("get %s: %v", repoName, err)
		} else if err = p.UpdateRepo(orgName, repoName, *repo); err != nil {
			return fmt.Errorf("update %s: %v", repoName, err)
		}
	}
	return nil
}

// UpdateRepo updates all branches in the repo with the specified defaults
func (p *protector) UpdateRepo(orgName string, repoName string, repo config.Repo) error {
	p.completedRepos[orgName+"/"+repoName] = true

	branches := map[string]github.Branch{}
	for _, onlyProtected := range []bool{false, true} { // put true second so b.Protected is set correctly
		bs, err := p.client.GetBranches(orgName, repoName, onlyProtected)
		if err != nil {
			return fmt.Errorf("list branches: %v", err)
		}
		for _, b := range bs {
			branches[b.Name] = b
		}
	}

	for bn, githubBranch := range branches {
		if branch, err := repo.GetBranch(bn); err != nil {
			return fmt.Errorf("get %s: %v", bn, err)
		} else if err = p.UpdateBranch(orgName, repoName, bn, *branch, githubBranch.Protected); err != nil {
			return fmt.Errorf("update %s from protected=%t: %v", bn, githubBranch.Protected, err)
		}
	}
	return nil
}

// UpdateBranch updates the branch with the specified configuration
func (p *protector) UpdateBranch(orgName, repo string, branchName string, branch config.Branch, protected bool) error {
	bp, err := p.cfg.GetPolicy(orgName, repo, branchName, branch)
	if err != nil {
		return fmt.Errorf("get policy: %v", err)
	}
	if bp == nil || bp.Protect == nil {
		return nil
	}
	if !protected && !*bp.Protect {
		logrus.Infof("%s/%s=%s: already unprotected", orgName, repo, branchName)
		return nil
	}
	var req *github.BranchProtectionRequest
	if *bp.Protect {
		r := makeRequest(*bp)
		req = &r
	}
	p.updates <- requirements{
		Org:     orgName,
		Repo:    repo,
		Branch:  branchName,
		Request: req,
	}
	return nil
}
