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

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"k8s.io/test-infra/maintenance/migratestatus/migrator"
)

func main() {
	var tokenFile, org, repo, copyContext, moveContext, retireContext, destContext string
	var continueOnError, dryRun bool

	flag.StringVar(&tokenFile, "tokenfile", "", "The file containing the token to use for authentication.")
	flag.StringVar(&org, "org", "", "The organization that owns the repo.")
	flag.StringVar(&repo, "repo", "", "The repo needing status migration.")
	flag.BoolVar(&dryRun, "dry-run", true, "Run in dry-run mode, performing no modifying actions.")
	flag.BoolVar(&continueOnError, "continue-on-error", false, "Indicates that the migration should continue if context migration fails for an individual PR.")

	flag.StringVar(&copyContext, "copy", "", "Indicates copy mode and specifies the context to copy.")
	flag.StringVar(&moveContext, "move", "", "Indicates move mode and specifies the context to move.")
	flag.StringVar(&retireContext, "retire", "", "Indicates retire mode and specifies the context to retire.")
	flag.StringVar(&destContext, "dest", "", "The destination context to copy or move to. For retire mode this is the context that replaced the retired context.")
	flag.Parse()

	if org == "" {
		errorfExit("'--org' must be set.\n")
	}
	if repo == "" {
		errorfExit("'--repo' must be set.\n")
	}
	if tokenFile == "" {
		errorfExit("'--tokenfile' must be set.\n")
	}
	tokenData, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		errorfExit("Error loading token file: %v\n", err)
	}

	if destContext == "" && retireContext == "" {
		errorfExit("'--dest' is required unless using '--retire' mode.\n")
	}

	var mode *migrator.Mode
	modeMsg := "Exactly one mode must be specified [--copy|--retire|--move].\n"
	if copyContext != "" {
		mode = migrator.CopyMode(copyContext, destContext)
	}
	if moveContext != "" {
		if mode != nil {
			errorfExit(modeMsg)
		}
		mode = migrator.MoveMode(moveContext, destContext)
	}
	if retireContext != "" {
		if mode != nil {
			errorfExit(modeMsg)
		}
		mode = migrator.RetireMode(retireContext, destContext)
	}
	if mode == nil {
		errorfExit(modeMsg)
	}

	// Note that continueOnError is false by default so that errors can be addressed when they occur
	// instead of blindly continuing to the next PR, possibly continuing to error.
	m := migrator.New(*mode, strings.TrimSpace(string(tokenData)), org, repo, dryRun, continueOnError)

	prOptions := &github.PullRequestListOptions{}
	if err := m.Migrate(prOptions); err != nil {
		errorfExit("Error during status migration: %v\n", err)
	}
	glog.Flush()
	os.Exit(0)
}

func errorfExit(format string, args ...interface{}) {
	glog.Errorf(format, args...)
	glog.Flush()
	os.Exit(1)
}
