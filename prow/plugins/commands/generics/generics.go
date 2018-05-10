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

package generics

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/errorutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/commands"
	"k8s.io/test-infra/prow/plugins/commands/handlers"
	"k8s.io/test-infra/prow/plugins/commands/policies"
)

const reFormatAddRemove = `(?mi)^/(remove-)?%s(?: +(.*?))?\s*$`

// AddRemoveCommands take the form `/[remove-]<command> [<args>]`.
func AddRemoveCommand(command string, issueType commands.IssueType, add, remove commands.MatchHandler) commands.Command {
	handler := func(ctx *commands.Context) error {
		var addMatches, removeMatches [][]string
		for _, match := range ctx.Matches {
			if len(match) != 3 {
				return fmt.Errorf("expected 3 regexp match groups, but got %d", len(match))
			}
			if strings.ToLower(match[1]) == "remove-" {
				removeMatches = append(removeMatches, match)
			} else {
				addMatches = append(addMatches, match)
			}
		}
		var err error
		if len(removeMatches) > 0 {
			ctx.Matches = removeMatches
			err = remove(ctx)
		}
		if len(addMatches) > 0 {
			ctx.Matches = addMatches
			err = errorutil.NewAggregate(err, add(ctx))
		}
		return err
	}
	return commands.New(
		issueType,
		// TODO: Change this to return an error when switching to dynamically defined plugins.
		regexp.MustCompile(fmt.Sprintf(reFormatAddRemove, command)),
		handler,
	)
}

type CommandHelpProvider func(*plugins.Configuration) pluginhelp.Command

func SingleLabelCommand(command, label string, issueType commands.IssueType, policy policies.AccessPolicy) (commands.Command, CommandHelpProvider) {
	return AddRemoveCommand(
			command,
			issueType,
			policies.Apply(policy, handlers.EnsureLabel(label)),
			policies.Apply(policy, handlers.RemoveLabel(label)),
		),
		func(config *plugins.Configuration) pluginhelp.Command {
			return pluginhelp.Command{
				Usage:       "/[remove-]" + command,
				Description: fmt.Sprintf("Adds or removes the %q label on %s.", label, issueType),
				Examples:    []string{"/" + command, "/remove-" + command},
				WhoCanUse:   policy.Who(config) + " can use this command.",
			}
		}
}
