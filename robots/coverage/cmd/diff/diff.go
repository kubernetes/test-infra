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

package diff

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/test-infra/gopherage/pkg/util"
	"k8s.io/test-infra/robots/coverage/diff"
)

type flags struct {
	outputFile string
	threshold  float32
	jobName    string
}

// MakeCommand returns a `diff` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "diff [base-profile] [new-profile]",
		Short: "Calculate the file level difference between two coverage profiles",
		Long: `Calculate the file level difference between two coverage profiles.
		Produce the result in a markdown table`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.outputFile, "output", "o", "-", "output file")
	cmd.Flags().StringVarP(&flags.jobName, "jobname", "j", "", "prow job name")
	cmd.Flags().Float32VarP(&flags.threshold, "threshold", "t", .8, "code coverage threshold")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Expected exactly two arguments: base-profile & new-profile")
		cmd.Usage()
		os.Exit(2)
	}

	if flags.threshold < 0 || flags.threshold > 1 {
		fmt.Fprintln(os.Stderr, "coverage threshold must be a float number between 0 to 1, inclusive")
		os.Exit(1)
	}

	baseProfilePath := args[0]
	newProfilePath := args[1]

	baseProfiles, err := util.LoadProfile(baseProfilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse base profile file: %v.\n", err)
		os.Exit(1)
	}

	newProfiles, err := util.LoadProfile(newProfilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse new profile file: %v.\n", err)
		os.Exit(1)
	}

	postContent, isCoverageLow := diff.ContentForGithubPost(baseProfiles, newProfiles, flags.jobName, flags.threshold)

	var file io.WriteCloser
	if flags.outputFile == "-" {
		file = os.Stdout
	} else {
		file, err = os.Create(flags.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output file: %v.", err)
			os.Exit(1)
		}
		defer file.Close()
	}

	_, err = io.WriteString(file, fmt.Sprintf("isCoverageLow = %v\n", isCoverageLow))

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write low coverage check: %v.\n", err)
		os.Exit(1)
	}

	_, err = io.WriteString(file, fmt.Sprintf("Post content:\n%v", postContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write post content: %v.\n", err)
		os.Exit(1)
	}
}
