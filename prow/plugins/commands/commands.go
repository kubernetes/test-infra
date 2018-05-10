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

/*
	Package commands provides a modular framework for defining command based Prow plugins.

	Example label based commands:
	(1) [Full Example] The `hold` plugin becomes mostly documentation:

	func init() {
		 cmd, cmdHelp := SingleLabelCommand("hold", "do-not-merge/hold", IssueTypePR, OpenAccess)
		 plugins.RegisterGenericCommentHandler("hold", cmd, helpProvider(cmdHelp))
	}

	func helpProvider(cmdHelp *pluginhelp.Command) plugins.HelpProvider {
		return func(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
			pluginHelp := &pluginhelp.PluginHelp{
				Description: "The hold plugin allows anyone to add or remove the '" + label + "' label from a pull request in order to temporarily prevent the PR from merging without withholding approval.",
			}
			pluginHelp.AddCommand(*cmdHelp)
			return pluginHelp, nil
		 }
	}

	(2) The `milestonestatus` plugin:

		cmd, cmdHelp := MultiLabelCommand("status", milestonestatus.statusMap, IssueTypeBoth, TeamAccess(teamId))
		plugins.RegisterGenericCommentHandler("milestonestatus", cmd, helpProvider(cmdHelp))

	(3) A `merge` plugin that gives the author control of a `mergeable` label:

		cmd, cmdHelp := SingleLabelCommand("merge", "mergeable", IssueTypePR, AuthorAccess)
		plugins.RegisterGenericCommentHandler("merge", cmd, helpProvider(cmdHelp))
*/
package commands

// TODO(cjwagner): allow dynamically adding new commands at runtime via config
// instead of code.

import (
	"regexp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

// IssueType specifies whether a command applies to issues, PRs, or both.
type IssueType string

func (i IssueType) String() string {
	return string(i)
}

const (
	IssueTypeIssue = "issues"
	IssueTypePR    = "pull requests"
	IssueTypeBoth  = "issues and pull requests"
)

// Command represents a single command based plugin and is the unit that can be
// registered to handle generic comments via the 'plugins' package.
type Command struct {
	issueType IssueType
	re        *regexp.Regexp

	// handler is called if the command regex matches a comment on an issue of the
	// correct type.
	handler MatchHandler
}

// New returns a new Command.
// This constructor exists to allow the struct fields to be package private.
// This prevents a plugin from changing the struct values after creation time
// which could race with their use in GenericCommentHandler.
func New(issueType IssueType, re *regexp.Regexp, handler MatchHandler) Command {
	return &Command{
		issueType: issueType,
		re:        re,
		handler:   handler,
	}
}

// Context provides MatchHandlers with a PluginClient, the regexp match, and
// data about the triggering event.
type Context struct {
	Client  *plugins.PluginClient
	Event   *github.GenericCommentEvent
	Matches [][]string
}

// MatchHandler reacts to a command match on the appropriate issue type.
// This is where the interesting plugin logic will live.
// See "k8s.io/test-infra/prow/plugins/commands/handlers" for some examples.
type MatchHandler func(*Context) error

// GenericCommentHandler checks for command regexp matches in freshly created
// comments on the appropriate issue type and calls the match handler if the
// regexp matches.
func (c Command) GenericCommentHandler(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	switch c.issueType {
	case IssueTypeIssue:
		if e.IsPR {
			return nil
		}
	case IssueTypePR:
		if !e.IsPR {
			return nil
		}
	}
	if matches := c.re.FindAllStringSubmatch(e.Body, -1); len(matches) > 0 {
		return c.handler(&Context{
			Client:  &pc,
			Event:   &e,
			Matches: matches,
		})
	}
	return nil
}
