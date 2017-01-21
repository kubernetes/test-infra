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
	"reflect"
	"testing"

	"github.com/google/go-github/github"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		expectedCommand *Command
		comment         string
	}{
		{
			expectedCommand: nil,
			comment:         "I have nothing to do with a command",
		},
		{
			expectedCommand: nil,
			comment:         " /COMMAND Line can't start with spaces",
		},
		{
			expectedCommand: nil,
			comment:         "Command not at the beginning:\n/COMMAND\nAnd something else...",
		},
		{
			expectedCommand: &Command{Name: "COMMAND"},
			comment:         "/COMMAND",
		},
		{
			expectedCommand: &Command{Name: "COMMAND", Arguments: "Valid command"},
			comment:         "/COMMAND Valid command",
		},
		{
			expectedCommand: &Command{Name: "COMMAND", Arguments: "Multiple Lines"},
			comment:         "/COMMAND Multiple Lines\nAnd something else...",
		},
		{
			expectedCommand: &Command{Name: "COMMAND", Arguments: "Args"},
			comment:         "/COMMAND Args\n/OTHERCOMMAND OtherArgs",
		},
		{
			expectedCommand: &Command{Name: "COMMAND", Arguments: "Command name is upper-cased"},
			comment:         "/command Command name is upper-cased",
		},
		{
			expectedCommand: &Command{Name: "COMMAND", Arguments: "Arguments is trimmed"},
			comment:         "/COMMAND     Arguments is trimmed   ",
		},
	}

	for _, test := range tests {
		actualCommand := ParseCommand(&github.IssueComment{Body: &test.comment})
		if !reflect.DeepEqual(actualCommand, test.expectedCommand) {
			t.Error(actualCommand, "doesn't match expected command:", test.expectedCommand)
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
