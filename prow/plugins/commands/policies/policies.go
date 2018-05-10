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

// Package policies defines an AccessPolicy interface that can be used to
// gate access to a MatchHandler and implements some common policies.
package policies

import (
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/commands"
)

// AccessPolicy describes an policy check.
type AccessPolicy interface {
	// CanAccess does the actual policy check and returns a value to indicate if
	// the MatchHandler should be run.
	CanAccess(*commands.Context) (bool, error)
	// Who provides a description of the users who are allowed.
	Who(config *plugins.Configuration) string
}

// Apply adds the specified policy check to the specified MatchHandler and
// returns a new MatchHandler that includes the policy check.
func Apply(policy AccessPolicy, handler commands.MatchHandler) commands.MatchHandler {
	return func(ctx *commands.Context) error {
		approved, err := policy.CanAccess(ctx)
		if err != nil {
			return err
		} else if approved {
			return handler(ctx)
		}
		// Commenter is not allowed to use the command.
		msg := fmt.Sprintf("You do not have permission to use this command.\n%s can use this command.", policy.Who(ctx.Client.PluginConfig))
		e := ctx.Event
		return ctx.Client.GitHubClient.CreateComment(
			e.Repo.Owner.Login,
			e.Repo.Name,
			e.Number,
			plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg),
		)
	}
}

type union []AccessPolicy

func Union(options ...AccessPolicy) AccessPolicy {
	return union(options)
}

func (a union) CanAccess(ctx *commands.Context) (bool, error) {
	for _, policy := range a {
		if allowed, err := policy.CanAccess(ctx); err != nil {
			return false, err
		} else if allowed {
			return true, nil
		}
	}
	return false, nil
}

func (a union) Who(config *plugins.Configuration) string {
	strs := make([]string, 0, len(a))
	for _, policy := range a {
		strs = append(strs, policy.Who(config))
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, " AND "))
}

/*
Additional possible policies:
- org member
- GH team indexed by org(/repo)
- issue/pr author
- OWNERS alias
- user whitelist
- only allow on friday the 13th
*/
type openAccess struct{}

var _ = AccessPolicy(openAccess{})

// OpenAccess allows anyone to use a command. This is the 'no-op' policy.
func OpenAccess() AccessPolicy {
	return openAccess{}
}

func (o openAccess) CanAccess(*commands.Context) (bool, error) {
	return true, nil
}

func (o openAccess) Who(*plugins.Configuration) string {
	return "Everyone"
}

// TeamAccess allows members of a specific GitHub team to use a command.
func TeamAccess(teamGetter func(*plugins.Configuration) (teamName string, teamID int)) AccessPolicy {
	return teamAccess(teamGetter)
}

type teamClient interface {
	ListTeamMembers(id int, role string) ([]github.TeamMember, error)
}

type teamAccess func(*plugins.Configuration) (teamName string, teamID int)

var _ = AccessPolicy(TeamAccess(nil))

func (t teamAccess) CanAccess(ctx *commands.Context) (bool, error) {
	return t.doCanAccess(ctx.Client.PluginConfig, ctx.Client.GitHubClient, ctx.Event.User.Login)
}

func (t teamAccess) doCanAccess(cfg *plugins.Configuration, client teamClient, login string) (bool, error) {
	name, id := t(cfg)
	members, err := client.ListTeamMembers(id, github.RoleAll)
	if err != nil {
		return false, fmt.Errorf("error listing members of team %q (ID: %d): %v", name, id, err)
	}

	login = github.NormLogin(login)
	for _, person := range members {
		if github.NormLogin(person.Login) == login {
			return true, nil
		}
	}
	return false, nil
}

func (t teamAccess) Who(config *plugins.Configuration) string {
	name, _ := t(config)
	return fmt.Sprintf("Members of the %q GitHub team", name)
}
