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

package junit

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/test-infra/gopherage/pkg/cov/junit"
	"k8s.io/test-infra/gopherage/pkg/util"
)

type flags struct {
	outputFile string
	threshold  float32
}

// MakeCommand returns a `junit` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "junit [profile]",
		Short: "Summarize coverage profile and produce the result in junit xml format.",
		Long: `Summarize coverage profile and produce the result in junit xml format.
Summary done at per-file and per-package level. Any coverage below coverage-threshold will be marked
with a <failure> tag in the xml produced.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.outputFile, "output", "o", "-", "output file")
	cmd.Flags().Float32VarP(&flags.threshold, "threshold", "t", .8, "code coverage threshold")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Expected exactly one argument: coverage file path")
		cmd.Usage()
		os.Exit(2)
	}

	if flags.threshold < 0 || flags.threshold > 1 {
		fmt.Fprintln(os.Stderr, "coverage threshold must be a float number between 0 to 1, inclusively")
		os.Exit(1)
	}

	profilePath := args[0]

	profiles, err := util.LoadProfile(profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse profile file: %v.", err)
		os.Exit(1)
	}

	text, err := junit.ProfileToTestsuiteXML(profiles, flags.threshold)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to produce xml from profiles: %v.", err)
		os.Exit(1)
	}

	var file io.WriteCloser
	if flags.outputFile == "-" {
		file = os.Stdout
	} else {
		file, err = os.Create(flags.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create file: %v.", err)
			os.Exit(1)
		}
		defer file.Close()
	}

	if _, err = file.Write(text); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write xml: %v.", err)
		os.Exit(1)
	}
}
