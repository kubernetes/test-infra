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

// Package mergeblocker implements the `/merge-blocker` command which manages
// the `merge-blocker` label on issues.
package mergeblocker

import (
	"fmt"

	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/commands"
	"k8s.io/test-infra/prow/plugins/commands/generics"
	"k8s.io/test-infra/prow/plugins/commands/policies"
)

const pluginName = "merge-blocker"

func init() {
	cmd, cmdProvider := generics.SingleLabelCommand(
		pluginName, // Command
		pluginName, // Label
		commands.IssueTypeIssue,
		policies.TeamAccess(func(config *plugins.Configuration) (string, int) {
			return config.MergeBlocker.TeamName, config.MergeBlocker.TeamID
		}),
	)
	plugins.RegisterGenericCommentHandler(pluginName, cmd.GenericCommentHandler, helpProvider(cmdProvider))
}

func helpProvider(cmdProvider generics.CommandHelpProvider) plugins.HelpProvider {
	return func(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
		pluginHelp := &pluginhelp.PluginHelp{
			Description: "The merge-blocker plugin provides the `/merge-blocker` command to manage the `merge-blocker` label on GitHub issues. Only members of the configured GitHub team may use this command.",
			Config: map[string]string{
				"": fmt.Sprintf("The configuration limits access to this command to members of the %q (ID: %d) GitHub team.", config.MergeBlocker.TeamName, config.MergeBlocker.TeamID),
			},
		}
		pluginHelp.AddCommand(cmdProvider(config))
		return pluginHelp, nil
	}
}
