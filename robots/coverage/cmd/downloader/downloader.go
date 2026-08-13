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

package downloader

import (
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	"io"
	"k8s.io/test-infra/robots/coverage/downloader"
	"os"

	"github.com/spf13/cobra"
)

type flags struct {
	outputFile       string
	artifactsDirName string
	profileName      string
}

// MakeCommand returns a `download` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "download [bucket] [prowjob]",
		Short: "Finds and downloads the coverage profile file from the latest healthy build",
		Long: `Finds and downloads the coverage profile file from the latest healthy build
stored in given gcs directory.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.outputFile, "output", "o", "-", "output file")
	cmd.Flags().StringVarP(&flags.artifactsDirName, "artifactsDir", "a", "artifacts", "artifact directory name in GCS")
	cmd.Flags().StringVarP(&flags.profileName, "profile", "p", "coverage-profile", "code coverage profile file name in GCS")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Expected exactly two arguments: bucket & prowjob")
		cmd.Usage()
		os.Exit(2)
	}

	bucket := args[0]
	prowjob := args[1]

	var file io.WriteCloser
	if flags.outputFile == "-" {
		file = os.Stdout
	} else {
		file, err := os.Create(flags.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output file: %v.", err)
			os.Exit(1)
		}
		defer file.Close()
	}
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create new storage client: %v.\n", err)
		os.Exit(1)
	}

	content, err := downloader.FindBaseProfile(ctx, client, bucket, prowjob, flags.artifactsDirName, flags.profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find base profile file: %v.\n", err)
		os.Exit(1)
	}
	_, err = file.Write(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write profile: %v.\n", err)
		os.Exit(1)
	}
}
