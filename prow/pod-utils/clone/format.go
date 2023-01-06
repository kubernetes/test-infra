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

package clone

import (
	"bytes"
	"fmt"
)

// FormatRecord describes the record in a human-readable
// manner for inclusion into build logs
func FormatRecord(record Record) string {
	var output bytes.Buffer
	if record.Failed {
		fmt.Fprintln(&output, "# FAILED")
	}
	if record.Refs.Repo == "" {
		fmt.Fprintf(&output, "Environment setup")
	} else {
		fmt.Fprintf(&output, "# Cloning %s/%s at %s", record.Refs.Org, record.Refs.Repo, record.Refs.BaseRef)
	}
	if record.Refs.BaseSHA != "" {
		fmt.Fprintf(&output, "(%s)", record.Refs.BaseSHA)
	}
	output.WriteString("\n")
	if len(record.Refs.Pulls) > 0 {
		output.WriteString("# Checking out pulls:\n")
		for _, pull := range record.Refs.Pulls {
			fmt.Fprintf(&output, "#\t%d", pull.Number)
			if pull.SHA != "" {
				fmt.Fprintf(&output, "(%s)", pull.SHA)
			}
			fmt.Fprint(&output, "\n")
		}
	}
	for _, command := range record.Commands {
		runtime := ""
		if command.Duration != 0 {
			runtime = fmt.Sprintf(" (runtime: %v)", command.Duration)
		}
		fmt.Fprintf(&output, "$ %s%s\n", command.Command, runtime)
		fmt.Fprint(&output, command.Output)
		if command.Error != "" {
			fmt.Fprintf(&output, "# Error: %s\n", command.Error)
		}
	}
	fmt.Fprintf(&output, "# Final SHA: %v\n", record.FinalSHA)
	fmt.Fprintf(&output, "# Total runtime: %v\n", record.Duration)

	return output.String()
}
