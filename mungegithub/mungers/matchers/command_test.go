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

package matchers

import (
	"reflect"
	"testing"

	"github.com/google/go-github/github"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		expectedCommands []*Command
		comment          string
	}{
		{
			expectedCommands: []*Command{},
			comment:          "I have nothing to do with a command",
		},
		{
			expectedCommands: []*Command{},
			comment:          " /COMMAND Must be at the beginning of the line",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND"}},
			comment:          "/COMMAND",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND"}},
			comment:          "/COMMAND\r",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Args after tab"}},
			comment:          "/COMMAND\tArgs after tab",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Removes trailing backslash R"}},
			comment:          "/COMMAND Removes trailing backslash R\r\n",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Valid command"}},
			comment:          "/COMMAND Valid command",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Multiple Lines"}},
			comment:          "/COMMAND Multiple Lines\nAnd something else...",
		},
		{
			expectedCommands: []*Command{
				{Name: "COMMAND", Arguments: "Args"},
				{Name: "OTHERCOMMAND", Arguments: "OtherArgs"},
			},
			comment: "/COMMAND Args\n/OTHERCOMMAND OtherArgs",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Command name is upper-cased"}},
			comment:          "/command Command name is upper-cased",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND", Arguments: "Arguments is trimmed"}},
			comment:          "/COMMAND     Arguments is trimmed   ",
		},
		{
			expectedCommands: []*Command{{Name: "COMMAND"}},
			comment:          "Command not at the beginning:\n/COMMAND\nAnd something else...",
		},
	}

	for _, test := range tests {
		actualCommand := ParseCommands(&github.IssueComment{Body: &test.comment})
		if !reflect.DeepEqual(actualCommand, test.expectedCommands) {
			t.Error(actualCommand, "doesn't match expected commands:", test.expectedCommands)
		}
	}
}

func TestStringCommand(t *testing.T) {
	tests := []struct {
		command        *Command
		expectedString string
	}{
		{
			command:        &Command{Name: "COMMAND", Arguments: "Argument"},
			expectedString: "/COMMAND Argument",
		},
		{
			command:        &Command{Name: "command", Arguments: "  Argument  "},
			expectedString: "/COMMAND Argument",
		},
		{
			command:        &Command{Name: "command"},
			expectedString: "/COMMAND",
		},
	}

	for _, test := range tests {
		actualString := test.command.String()
		if actualString != test.expectedString {
			t.Error(actualString, "doesn't match expected string:", test.expectedString)
		}
	}
}
