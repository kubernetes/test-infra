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

package comment

import (
	"regexp"
	"strings"
)

// Command is a way for human to interact with the bot
type Command struct {
	Name      string
	Arguments string
}

var (
	// Matches a command:
	// - Line that starts with slash
	// - followed by non-space characteres,
	// - (optional) followed by space and arguments
	commandRegex = regexp.MustCompile(`(?m)^/([^\s]+)[\t ]*([^\n\r]*)`)
)

// ParseCommands attempts to read a command from a comment
// Returns nil if the comment doesn't contain a command
func ParseCommands(comment *Comment) []*Command {
	if comment == nil || comment.Body == nil {
		return nil
	}

	commands := []*Command{}
	matches := commandRegex.FindAllStringSubmatch(*comment.Body, -1)
	for _, match := range matches {
		commands = append(commands, &Command{
			Name:      strings.ToUpper(match[1]),
			Arguments: strings.TrimSpace(match[2]),
		})
	}

	return commands
}

// String displays the command
func (n *Command) String() string {
	str := "/" + strings.ToUpper(n.Name)
	args := strings.TrimSpace(n.Arguments)
	if args != "" {
		str += " " + args
	}
	return str
}
