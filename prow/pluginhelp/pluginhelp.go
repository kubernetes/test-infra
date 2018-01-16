/*
Copyright 2017 The Kubernetes Authors.

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

// Package pluginhelp defines structures that represent plugin help information.
// These structs are used by sub-packages 'hook' and 'externalplugins'.
package pluginhelp

// Command is a serializable representation of the command information for a single command.
type Command struct {
	// Frequency suggests how often a command is used. Value ranges from 0 to 2, the SMALLER the
	// number is, the more often the command is used.
	Frequency int
	// Description is a short description about what does the command do.
	Description string
	// Examples is a list of usage example for the command.
	Examples []string
}

// PluginHelp is a serializable representation of the help information for a single plugin.
// This includes repo specific configuration for every repo that the plugin is enabled for.
type PluginHelp struct {
	// Description is a description of what the plugin does and what purpose it achieves.
	// This field may include HTML.
	Description string
	// WhoCanUse is a description of the permissions/role/authorization required to use the plugin.
	// This is usually specified as a github permissions, but it can also be a github team, an
	// OWNERS file alias, etc.
	// This field may include HTML.
	WhoCanUse string
	// Usage is a usage string for the plugin. Leave empty if not applicable.
	Usage string
	// Examples is a list of usage examples for the plugin. Leave empty if not applicable.
	Examples []string
	// Config is a map from org/repo strings to a string describing the configuration for that repo.
	// The key "" should map to a string describing configuration that applies to all repos if any.
	// This configuration strings may include HTML.
	Config map[string]string

	// Events is a slice containing the events that are handled by the plugin.
	// NOTE: Plugins do not need to populate this. Hook populates it on their behalf.
	Events []string
	// Commands maps a command name to a struct of its properties.
	Commands map[string]Command
}

// Help is a serializable representation of all plugin help information.
type Help struct {
	// AllRepos is a flatten (all org/repo, no org strings) list of the repos that use plugins.
	AllRepos []string
	// RepoPlugins maps org and org/repo strings to the plugins configured for that scope.
	// NOTE: The key "" maps to the list of all existing plugins (including failed help providers).
	// A mix of org or org/repo strings is desirable over the flattened form (all org/repo) that
	// 'AllRepos' uses because it matches the plugin configuration and is more human readable.
	RepoPlugins         map[string][]string
	RepoExternalPlugins map[string][]string
	// PluginHelp is maps plugin names to their help info.
	PluginHelp         map[string]PluginHelp
	ExternalPluginHelp map[string]PluginHelp
}

func (pluginHelp *PluginHelp) addCommand(name string, command Command) {
	pluginHelp.Commands[name] = command
}
