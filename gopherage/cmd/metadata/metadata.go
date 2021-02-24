/*
Copyright 2021 The Kubernetes Authors.

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

package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// From https://github.com/kubernetes/test-infra/blob/master/prow/jobs.md#job-environment-variables
	base_ref   = "PULL_BASE_REF"
	repo_owner = "REPO_OWNER"
)

type flags struct {
	outputFile    string
	host          string
	project       string
	commitID      string
	ref           string
	workspaceRoot string
}

type CoverageMetadata struct {
	Host     string `json:"host"`
	Project  string `json:"project"`
	Root     string `json:"workspace_root"`
	Ref      string `json:"ref"`
	CommitID string `json:"commit_id"`
}

// MakeCommand returns a `junit` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "metadata [...fields]",
		Short: "Produce json file containing metadata about coverage collection.",
		Long:  `Builds a json file containing information about the repo .`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.outputFile, "output", "o", "-", "output file")
	cmd.Flags().StringVarP(&flags.host, "host", "", "", "Name of repo host")
	cmd.Flags().StringVarP(&flags.project, "project", "p", "", "Project name")
	cmd.Flags().StringVarP(&flags.commitID, "commit", "c", "", "Current Commit Hash (git rev-parse HEAD)")
	cmd.Flags().StringVarP(&flags.ref, "ref", "r", "", "Current branch ref (git branch --show-current).")
	cmd.Flags().StringVarP(&flags.workspaceRoot, "root", "w", "", "path to root of repo")
	return cmd
}

func gitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if flags.project == "" {
		project := os.Getenv(repo_owner)
		if project == "" {
			fmt.Fprintf(os.Stdout, "Failed to collect project from ENV: (%s) not found", repo_owner)
			cmd.Usage()
			os.Exit(1)
		}
		flags.project = project
	}

	if flags.commitID == "" {
		commit, err := gitCommand("rev-parse", "HEAD")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch Commit Hash from within covered repo: %v.", err)
			os.Exit(1)
		}
		flags.commitID = commit
	}

	if flags.ref == "" {
		ref, err := gitCommand("branch", "--show-current")

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch ref from within covered repo: %v.", err)
			os.Exit(1)
		}
		flags.ref = ref
	}

	metadata := &CoverageMetadata{
		Host:     flags.host,
		Project:  flags.project,
		Root:     flags.workspaceRoot,
		Ref:      flags.ref,
		CommitID: flags.commitID,
	}

	j, err := json.Marshal(metadata)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build json: %v.", err)
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
	}
	if _, err := file.Write(j); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write json: %v.", err)
		os.Exit(1)
	}
	file.Close()
}
