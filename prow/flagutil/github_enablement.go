/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"flag"
	"fmt"
	"strings"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// GitHubEnablementOptions allows enable/disable functionality on a github org or
// org/repo level. If either EnabledOrgs or EnabledRepos is set, only org/repos in
// those are allowed, otherwise everything that is not in DisabledOrgs or DisabledRepos
// is allowed.
type GitHubEnablementOptions struct {
	enabledOrgs   Strings
	enabledRepos  Strings
	disabledOrgs  Strings
	disabledRepos Strings
}

func (o *GitHubEnablementOptions) AddFlags(fs *flag.FlagSet) {
	fs.Var(&o.enabledOrgs, "github-enabled-org", "Enabled github org. Can be passed multiple times. If set, all orgs or repos that are not allowed via --gitbub-enabled-orgs or --github-enabled-repos will be ignored")
	fs.Var(&o.enabledRepos, "github-enabled-repo", "Enabled github repo in org/repo format. Can be passed multiple times. If set, all orgs or repos that are not allowed via --gitbub-enabled-orgs or --github-enabled-repos will be ignored")
	fs.Var(&o.disabledOrgs, "github-disabled-org", "Disabled github org. Can be passed multiple times. Orgs that are in this list will be ignored.")
	fs.Var(&o.disabledRepos, "github-disabled-repo", "Disabled github repo in org/repo format. Can be passed multiple times. Repos that are in this list will be ignored.")
}

func (o *GitHubEnablementOptions) Validate(_ bool) error {
	var errs []error

	for _, enabledRepo := range o.enabledRepos.vals {
		if err := validateOrgRepoFormat(enabledRepo); err != nil {
			errs = append(errs, fmt.Errorf("--github-enabled-repo=%s is invalid: %w", enabledRepo, err))
		}
	}
	for _, disabledRepo := range o.disabledRepos.vals {
		if err := validateOrgRepoFormat(disabledRepo); err != nil {
			errs = append(errs, fmt.Errorf("--github-disabled-repo=%s is invalid: %w", disabledRepo, err))
		}
	}

	if intersection := o.enabledOrgs.StringSet().Intersection(o.disabledOrgs.StringSet()); len(intersection) != 0 {
		errs = append(errs, fmt.Errorf("%v is in both --github-enabled-org and --github-disabled-org", intersection.List()))
	}

	if intersection := o.enabledRepos.StringSet().Intersection(o.disabledRepos.StringSet()); len(intersection) != 0 {
		errs = append(errs, fmt.Errorf("%v is in both --github-enabled-repo and --github-disabled-repo", intersection.List()))
	}

	return utilerrors.NewAggregate(errs)
}

func validateOrgRepoFormat(orgRepo string) error {
	components := strings.Split(orgRepo, "/")
	if n := len(components); n != 2 || components[0] == "" || components[1] == "" {
		return fmt.Errorf("%q is not in org/repo format", orgRepo)
	}

	return nil
}

func (o *GitHubEnablementOptions) EnablementChecker() func(org, repo string) bool {
	enabledOrgs := o.enabledOrgs.StringSet()
	enabledRepos := o.enabledRepos.StringSet()
	disabledOrgs := o.disabledOrgs.StringSet()
	diabledRepos := o.disabledRepos.StringSet()
	return func(org, repo string) bool {
		if len(enabledOrgs) > 0 || len(enabledRepos) > 0 {
			if !enabledOrgs.Has(org) && !enabledRepos.Has(org+"/"+repo) {
				return false
			}
		}

		return !disabledOrgs.Has(org) && !diabledRepos.Has(org+"/"+repo)
	}
}
