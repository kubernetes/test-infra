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

package filter

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"k8s.io/test-infra/gopherage/pkg/cov"
	"k8s.io/test-infra/gopherage/pkg/util"
)

type flags struct {
	OutputFile   string
	IncludePaths []string
	ExcludePaths []string
}

// MakeCommand returns a `filter` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "filter [file]",
		Short: "Filters a Go coverage file.",
		Long:  `Filters a Go coverage file, removing entries that do not match the given flags.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.OutputFile, "output", "o", "-", "output file")
	cmd.Flags().StringSliceVar(&flags.IncludePaths, "include-path", nil, "If specified at least once, only files with paths matching one of these regexes are included.")
	cmd.Flags().StringSliceVar(&flags.ExcludePaths, "exclude-path", nil, "Files with paths matching one of these regexes are excluded. Can be used repeatedly.")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Expected one file.")
		cmd.Usage()
		os.Exit(2)
	}

	input, err := util.LoadProfile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't load %s: %v.", args[0], err)
		os.Exit(1)
	}

	output := input
	if len(flags.IncludePaths) > 0 {
		output, err = cov.FilterProfilePaths(output, flags.IncludePaths, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't filter by include paths: %v.", err)
		}
	}

	if len(flags.ExcludePaths) > 0 {
		output, err = cov.FilterProfilePaths(output, flags.ExcludePaths, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't filter by exclude paths: %v.", err)
		}
	}

	if err := util.DumpProfile(flags.OutputFile, output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
