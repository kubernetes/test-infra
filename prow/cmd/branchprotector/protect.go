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
	"io/ioutil"
	"log"
	"net/url"
	"strings"
	"sync"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/branchprotection"
	"k8s.io/test-infra/prow/github"
)

type options struct {
	config   string
	token    string
	confirm  bool
	endpoint string
}

func (o *options) Validate() error {
	if o.config == "" {
		return errors.New("empty --config")
	}

	if o.token == "" {
		return errors.New("empty --github-token-path")
	}

	if _, err := url.Parse(o.endpoint); err != nil {
		return fmt.Errorf("invalid --endpoint URL: %v", err)
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.config, "config-path", "", "Path to prow config.yaml")
	flag.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flag.StringVar(&o.endpoint, "github-endpoint", "https://api.github.com", "Github api endpoint, may differ for enterprise")
	flag.StringVar(&o.token, "github-token-path", "", "Path to github token")
	flag.Parse()
	return o
}

func jobRequirements(jobs []config.Presubmit, after bool) []string {
	var required []string
	for _, j := range jobs {
		// Does this job require a context or have kids that might need one?
		if !after && !j.AlwaysRun && j.RunIfChanged == "" {
			continue // No
		}
		if !j.SkipReport { // This job needs a context
			required = append(required, j.Context)
		}
		// Check which children require contexts
		required = append(required, jobRequirements(j.RunAfterSuccess, true)...)
	}
	return required
}

func repoRequirements(org, repo string, cfg config.Config) []string {
	p, ok := cfg.Presubmits[org+"/"+repo]
	if !ok {
		return nil
	}
	return jobRequirements(p, false)
}

type Requirements struct {
	Org    string
	Repo   string
	Branch string
	branchprotection.Policy
}

type Errors struct {
	lock sync.Mutex
	errs []error
}

func (e *Errors) add(err error) {
	e.lock.Lock()
	log.Println(err)
	defer e.lock.Unlock()
	e.errs = append(e.errs, err)
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load(o.config)
	if err != nil {
		log.Fatalf("Failed to load --config=%s: %v", o.config, err)
	}

	b, err := ioutil.ReadFile(o.token)
	if err != nil {
		log.Fatalf("cannot read --token: %v", err)
	}

	var c *github.Client
	tok := strings.TrimSpace(string(b))
	if o.confirm {
		c = github.NewClient(tok, o.endpoint)
	} else {
		c = github.NewDryRunClient(tok, o.endpoint)
	}
	c.Throttle(300, 100) // 300 hourly tokens, bursts of 100

	p := Protector{
		client:         c,
		cfg:            cfg,
		updates:        make(chan Requirements),
		errors:         Errors{},
		completedRepos: make(map[string]bool),
		done:           make(chan []error),
	}

	go p.ConfigureBranches()
	p.Protect()
	close(p.updates)
	errors := <-p.done
	if len(errors) > 0 {
		log.Fatalf("Encountered %d errors protecting branches: %v", len(errors), errors)
	}
}

type client interface {
	RemoveBranchProtection(org, repo, branch string) error
	UpdateBranchProtection(org, repo, branch string, policy github.BranchProtectionRequest) error
	GetBranches(org, repo string) ([]github.Branch, error)
	GetRepos(org string, user bool) ([]github.Repo, error)
}

type Protector struct {
	client         client
	cfg            *config.Config
	updates        chan Requirements
	errors         Errors
	completedRepos map[string]bool
	done           chan []error
}

func (p *Protector) ConfigureBranches() {
	for r := range p.updates {
		if r.Policy.Protect == nil {
			p.errors.add(fmt.Errorf("%s/%s:%s has Protect==nil", r.Org, r.Repo, r.Branch))
			continue
		}
		if !*r.Policy.Protect {
			err := p.client.RemoveBranchProtection(r.Org, r.Repo, r.Branch)
			if err != nil {
				p.errors.add(fmt.Errorf("remove %s/%s=%s protection failed: %v", r.Org, r.Repo, r.Branch, err))
			}
		} else {
			err := p.client.UpdateBranchProtection(r.Org, r.Repo, r.Branch, makeRequest(r.Policy))
			if err != nil {
				p.errors.add(fmt.Errorf("update %s/%s=%s protection failed: %v", r.Org, r.Repo, r.Branch, err))
			}
		}
	}
	p.done <- p.errors.errs
}

// Protect branches specified in the presubmit and branch-protection config sections.
func (p *Protector) Protect() {
	bp := p.cfg.BranchProtection

	policy, dep, err := bp.MakePolicy()
	if err != nil {
		p.errors.add(err)
		return
	}
	if dep {
		log.Println("WARNING: global branch-protection config uses deprecated fields (only policy, protect-tested-repos, orgs keys supported)")
	}

	if bp.ProtectTested { // Automatically require any required prow jobs for unconfigured repos
		for repoPath := range p.cfg.Presubmits {
			parts := strings.Split(repoPath, "/")
			if len(parts) != 2 {
				p.errors.add(fmt.Errorf("bad repo in prow jobs: %s", repoPath))
				continue
			}
			orgName := parts[0]
			repoName := parts[1]
			repoReqs := repoRequirements(orgName, repoName, *p.cfg)
			org, ok := bp.Orgs[orgName]
			if !ok {
				org = branchprotection.Org{
					Repos: map[string]branchprotection.Repo{},
				}
				bp.Orgs[orgName] = org
			}
			if _, ok = org.Repos[repoName]; !ok {
				protected := len(repoReqs) > 0
				repoPolicy := branchprotection.Policy{
					Protect: &protected,
				}
				if protected {
					repoPolicy.ContextPolicy = &branchprotection.ContextPolicy{
						Contexts: repoReqs,
					}
				}
				repo := branchprotection.Repo{
					Policy: repoPolicy,
				}
				org.Repos[repoName] = repo
			}

		}
	}

	// Scan the branch-protection configuration
	for orgName, org := range bp.Orgs {
		if err := p.updateOrg(orgName, org, policy); err != nil {
			p.errors.add(err)
		}
	}
}

// defined returns true if the policy's protect field is non-nil
func defined(p *branchprotection.Policy) bool {
	return p != nil && p.Protect != nil
}

// updateOrg will configure repos.
//
// It will update every org in the repo if an (inherited) protection policy is defined for the org.
// Otherwise it will only update explicitly defined repositories.
func (p *Protector) updateOrg(orgName string, org branchprotection.Org, defaultPolicy *branchprotection.Policy) error {

	orgPolicy, dep, err := org.MakePolicy()
	if err != nil {
		return fmt.Errorf("branch-protection.orgs[\"%s\"] mixes policy with deprecated fields: %v", orgName, err)
	}
	if dep {
		log.Printf("WARNING: %s branch protection config uses deprecated fields (only repos and policy keys supported)", orgName)
	}
	policy := branchprotection.MergePolicy(defaultPolicy, orgPolicy)

	var repos []string
	if defined(policy) {
		// Strongly opinionated org, configure every repo in the org.
		rs, err := p.client.GetRepos(orgName, false)
		if err != nil {
			return fmt.Errorf("GetRepos(%s) failed: %v", orgName, err)
		}
		for _, r := range rs {
			repos = append(repos, r.Name)
		}
	} else {
		// Unopinionated org, configure only explicitly listed repos
		for r := range org.Repos {
			repos = append(repos, r)
		}
	}

	for _, repoName := range repos {
		err := p.updateRepo(orgName, repoName, org.Repos[repoName], policy)
		if err != nil {
			return err
		}
	}
	return nil
}

// updateRepo will configure branches.
//
// It will update every branch in the repo if an (inherited) protection policy is defined for the repo.
// Otherwise it will only update explicitly defined branches.
func (p *Protector) updateRepo(orgName string, repo string, repoDefaults branchprotection.Repo, defaultPolicy *branchprotection.Policy) error {
	repoPolicy, dep, err := repoDefaults.MakePolicy()
	if err != nil {
		return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"] mixes policy with deprecated fields: %v", orgName, repo, err)
	}
	if dep {
		log.Printf("WARNING: %s/%s branch protection config uses deprecated fields (only branches and policy keys supported)", orgName, repo)
	}
	policy := branchprotection.MergePolicy(defaultPolicy, repoPolicy)
	p.completedRepos[orgName+"/"+repo] = true

	var branches []string
	if defined(policy) { // Opinionated org or repo, confiugre all branches
		bs, err := p.client.GetBranches(orgName, repo)
		if err != nil {
			return fmt.Errorf("GetBranches(%s, %s) failed: %v", orgName, repo, err)
		}
		for _, branch := range bs {
			branches = append(branches, branch.Name)
		}
	} else {
		for b := range repoDefaults.Branches {
			branches = append(branches, b)
		}
	}

	for _, branchName := range branches {
		if err := p.updateBranch(orgName, repo, branchName, repoDefaults.Branches[branchName], policy); err != nil {
			return fmt.Errorf("updateBranch(%s, %s, %s, ...) failed: %v", orgName, repo, branchName, err)
		}
	}
	return nil
}

// updateBranch configures branch protection is a protection policy is defined.
func (p *Protector) updateBranch(orgName, repo string, branchName string, branchDefaults branchprotection.Branch, defaultPolicy *branchprotection.Policy) error {

	branchPolicy, dep, err := branchDefaults.MakePolicy()
	if err != nil {
		return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"] uses both deprecated and non-deprecated fields: %v", orgName, repo, branchName, err)
	}
	if dep {
		log.Printf("WARNING: %s/%s=%s branch protection config uses deprecated fields (protect-by-default, require-contexts, allow-push are deprecated)", orgName, repo, branchName)
	}
	policy := branchprotection.MergePolicy(defaultPolicy, branchPolicy)
	if !defined(policy) {
		return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"] must specify a protect policy", orgName, repo, branchName)
	}
	if !*policy.Protect {
		// Otherwise ensure all settings are off
		switch {
		case policy.ContextPolicy != nil:
			return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"]: required_status_checks require protect", orgName, repo, branchName)
		case policy.Restrictions != nil:
			return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"]: restrictions require protect=true", orgName, repo, branchName)
		case policy.Admins != nil:
			return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"]: enforce_admins require protect=true", orgName, repo, branchName)
		case policy.ReviewPolicy != nil:
			return fmt.Errorf("branch-protection.orgs[\"%s\"].repos[\"%s\"].branches[\"%s\"]: required_pull_request_reviews require protect=true", orgName, repo, branchName)
		}
	}

	p.updates <- Requirements{
		Org:    orgName,
		Repo:   repo,
		Branch: branchName,
		Policy: *policy,
	}
	return nil
}
