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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// From https://github.com/kubernetes/test-infra/blob/master/prow/jobs.md#job-environment-variables
	repo_owner = "REPO_OWNER"
)

type Flags struct {
	outputFile    string
	host          string
	project       string
	workspaceRoot string
	traceType     string
	commitID      string
	ref           string
	source        string
	replace       string
	patchSet      string
	changeNum     string
}

type Metadata interface{}

type gitRunner func(...string) (string, error)
type envFetcher func(string) string

type CoverageMetadata struct {
	Host      string `json:"host"`
	Project   string `json:"project"`
	Root      string `json:"workspace_root"`
	TraceType string `json:"trace_type"`
}

type AbsMetadata struct {
	CoverageMetadata
	CommitID    string `json:"commit_id"`
	Ref         string `json:"ref"`
	Source      string `json:"source"`
	ReplaceRoot string `json:"git_project"`
}

type IncMetadata struct {
	CoverageMetadata
	ChangeNum string `json:"changelist_num"`
	PatchSet  string `json:"patchset_num"`
}

// MakeCommand returns a `junit` command.
func MakeCommand() *cobra.Command {
	Flags := &Flags{}
	baseCmd := &cobra.Command{
		Use:   "metadata [...fields]",
		Short: "Produce json file containing metadata about coverage collection.",
		Long:  `Builds a json file containing information about the repo .`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Sub command required [inc, abs]")
			os.Exit(1)
		},
	}

	absCmd := &cobra.Command{
		Use:   "abs [...fields]",
		Short: "Build abs metadata file.",
		Long:  "Produce json file containing metadata about absremental coverage collection.",
		Run: func(cmd *cobra.Command, args []string) {
			ValidateBase(Flags, cmd, os.Getenv)
			ValidateAbs(Flags, gitCommand)
			metadata := &AbsMetadata{
				CoverageMetadata: CoverageMetadata{
					Host:      Flags.host,
					Project:   Flags.project,
					Root:      Flags.workspaceRoot,
					TraceType: Flags.traceType,
				},
				CommitID:    Flags.commitID,
				Ref:         Flags.ref,
				Source:      Flags.source,
				ReplaceRoot: Flags.replace,
			}
			WriteJson(Flags, metadata)
		},
	}
	absCmd.Flags().StringVarP(&Flags.outputFile, "output", "o", "-", "output file")
	absCmd.Flags().StringVarP(&Flags.host, "host", "", "", "Name of repo host")
	absCmd.Flags().StringVarP(&Flags.project, "project", "p", "", "Project name")
	absCmd.Flags().StringVarP(&Flags.workspaceRoot, "root", "w", "", "path to workspace root of repo")
	absCmd.Flags().StringVarP(&Flags.traceType, "trace", "t", "COV", "type of coverage [COV, LCOV]")
	absCmd.Flags().StringVarP(&Flags.commitID, "commit", "c", "", "Current Commit Hash (git rev-parse HEAD)")
	absCmd.Flags().StringVarP(&Flags.ref, "ref", "r", "", "Current branch ref (git branch --show-current).")
	absCmd.Flags().StringVarP(&Flags.source, "source", "s", "", "custom field for information about coverage source")
	absCmd.Flags().StringVarP(&Flags.replace, "replace_root", "", "", "path to replace root of coverage paths with")

	incCmd := &cobra.Command{
		Use:   "inc [...fields]",
		Short: "Build inc metadata file.",
		Long:  "Produce json file containing metadata about incremental coverage collection.",
		Run: func(cmd *cobra.Command, args []string) {
			ValidateBase(Flags, cmd, os.Getenv)
			ValidateInc(Flags)
			metadata := &IncMetadata{
				CoverageMetadata: CoverageMetadata{
					Host:      Flags.host,
					Project:   Flags.project,
					Root:      Flags.workspaceRoot,
					TraceType: Flags.traceType,
				},
				ChangeNum: Flags.changeNum,
				PatchSet:  Flags.patchSet,
			}
			WriteJson(Flags, metadata)
		},
	}
	incCmd.Flags().StringVarP(&Flags.outputFile, "output", "o", "-", "output file")
	incCmd.Flags().StringVarP(&Flags.host, "host", "", "", "Name of repo host")
	incCmd.Flags().StringVarP(&Flags.project, "project", "p", "", "Project name")
	incCmd.Flags().StringVarP(&Flags.workspaceRoot, "root", "w", "", "path to workspace root of repo")
	incCmd.Flags().StringVarP(&Flags.traceType, "trace", "t", "COV", "type of coverage [COV, LCOV]")
	incCmd.Flags().StringVarP(&Flags.changeNum, "changelist_num", "", "", "Gerrit change number")
	incCmd.Flags().StringVarP(&Flags.patchSet, "patchset_num", "", "", "Gerrit Patchset Number")

	baseCmd.AddCommand(incCmd, absCmd)

	return baseCmd
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

func ValidateBase(Flags *Flags, cmd *cobra.Command, env envFetcher) error {
	if Flags.project == "" {
		project := env(repo_owner)
		if project == "" {
			cmd.Usage()
			return fmt.Errorf("Failed to collect project from ENV: (%s) not found", repo_owner)
		}
		Flags.project = project
	}

	return nil
}

func ValidateInc(Flags *Flags) error {
	if Flags.changeNum == "" {
		return errors.New("Gerrit change number is required")
	}

	if Flags.patchSet == "" {
		return errors.New("Gerrit patchset number is required")
	}
	return nil
}

func ValidateAbs(Flags *Flags, r gitRunner) error {
	if Flags.commitID == "" {
		commit, err := r("rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("Failed to fetch Commit Hash from within covered repo: %v.", err)
		}
		Flags.commitID = commit
	}

	if Flags.ref == "" {
		ref, err := r("branch", "--show-current")

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch ref from within covered repo: %v. Defaulting to HEAD", err)
			ref = "HEAD"
		}
		Flags.ref = ref
	}
	return nil
}

func WriteJson(Flags *Flags, m Metadata) error {
	var file io.WriteCloser

	j, err := json.Marshal(m)

	if err != nil {
		return fmt.Errorf("Failed to build json: %v.", err)
	}

	if Flags.outputFile == "-" {
		file = os.Stdout
	} else {
		file, err = os.Create(Flags.outputFile)
		if err != nil {
			return fmt.Errorf("Failed to create file: %v.", err)
		}
	}
	if _, err := file.Write(j); err != nil {
		return fmt.Errorf("Failed to write json: %v.", err)
	}
	file.Close()
	return nil
}
