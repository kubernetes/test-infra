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

package clone

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
)

// Run clones the refs under the prescribed directory and optionally
// configures the git username and email in the repository as well.
func Run(refs kube.Refs, dir, gitUserName, gitUserEmail, cookiePath string, env []string) Record {
	logrus.WithFields(logrus.Fields{"refs": refs}).Info("Cloning refs")
	record := Record{Refs: refs}

	// This function runs the provided commands in order, logging them as they run,
	// aborting early and returning if any command fails.
	runCommands := func(commands []cloneCommand) error {
		for _, command := range commands {
			formattedCommand, output, err := command.run()
			logrus.WithFields(logrus.Fields{"command": formattedCommand, "output": output, "error": err}).Info("Ran command")
			message := ""
			if err != nil {
				message = err.Error()
				record.Failed = true
			}
			record.Commands = append(record.Commands, Command{Command: formattedCommand, Output: output, Error: message})
			if err != nil {
				return err
			}
		}
		return nil
	}

	g := gitCtxForRefs(refs, dir, env)
	if err := runCommands(g.commandsForBaseRef(refs, gitUserName, gitUserEmail, cookiePath)); err != nil {
		return record
	}
	timestamp, err := g.gitHeadTimestamp()
	if err != nil {
		timestamp = int(time.Now().Unix())
	}
	if err := runCommands(g.commandsForPullRefs(refs, timestamp)); err != nil {
		return record
	}
	return record
}

// PathForRefs determines the full path to where
// refs should be cloned
func PathForRefs(baseDir string, refs kube.Refs) string {
	var clonePath string
	if refs.PathAlias != "" {
		clonePath = refs.PathAlias
	} else {
		clonePath = fmt.Sprintf("github.com/%s/%s", refs.Org, refs.Repo)
	}
	return fmt.Sprintf("%s/src/%s", baseDir, clonePath)
}

// gitCtx collects a few common values needed for all git commands.
type gitCtx struct {
	cloneDir      string
	env           []string
	repositoryURI string
}

// gitCtxForRefs creates a gitCtx based on the provide refs and baseDir.
func gitCtxForRefs(refs kube.Refs, baseDir string, env []string) gitCtx {
	g := gitCtx{
		cloneDir:      PathForRefs(baseDir, refs),
		env:           env,
		repositoryURI: fmt.Sprintf("https://github.com/%s/%s.git", refs.Org, refs.Repo),
	}
	if refs.CloneURI != "" {
		g.repositoryURI = refs.CloneURI
	}
	return g
}

func (g *gitCtx) gitCommand(args ...string) cloneCommand {
	return cloneCommand{dir: g.cloneDir, env: g.env, command: "git", args: args}
}

// commandsForBaseRef returns the list of commands needed to initialize and
// configure a local git directory, as well as fetch and check out the provided
// base ref.
func (g *gitCtx) commandsForBaseRef(refs kube.Refs, gitUserName, gitUserEmail, cookiePath string) []cloneCommand {
	commands := []cloneCommand{{dir: "/", env: g.env, command: "mkdir", args: []string{"-p", g.cloneDir}}}

	commands = append(commands, g.gitCommand("init"))
	if gitUserName != "" {
		commands = append(commands, g.gitCommand("config", "user.name", gitUserName))
	}
	if gitUserEmail != "" {
		commands = append(commands, g.gitCommand("config", "user.email", gitUserEmail))
	}
	if cookiePath != "" {
		commands = append(commands, g.gitCommand("config", "http.cookiefile", cookiePath))
	}
	commands = append(commands, g.gitCommand("fetch", g.repositoryURI, "--tags", "--prune"))
	commands = append(commands, g.gitCommand("fetch", g.repositoryURI, refs.BaseRef))

	var target string
	if refs.BaseSHA != "" {
		target = refs.BaseSHA
	} else {
		target = "FETCH_HEAD"
	}
	// we need to be "on" the target branch after the sync
	// so we need to set the branch to point to the base ref,
	// but we cannot update a branch we are on, so in case we
	// are on the branch we are syncing, we check out the SHA
	// first and reset the branch second, then check out the
	// branch we just reset to be in the correct final state
	commands = append(commands, g.gitCommand("checkout", target))
	commands = append(commands, g.gitCommand("branch", "--force", refs.BaseRef, target))
	commands = append(commands, g.gitCommand("checkout", refs.BaseRef))

	return commands
}

// gitHeadTimestamp returns the timestamp of the HEAD commit as seconds from the
// UNIX epoch. If unable to read the timestamp for any reason (such as missing
// the git, or not using a git repo), it returns 0 and an error.
func (g *gitCtx) gitHeadTimestamp() (int, error) {
	gitShowCommand := g.gitCommand("show", "-s", "--format=format:%ct", "HEAD")
	_, gitOutput, err := gitShowCommand.run()
	if err != nil {
		logrus.WithError(err).Debug("Could not obtain timestamp of git HEAD")
		return 0, err
	}
	timestamp, convErr := strconv.Atoi(string(gitOutput))
	if convErr != nil {
		logrus.WithError(convErr).Errorf("Failed to parse timestamp %q", gitOutput)
		return 0, convErr
	}
	return timestamp, nil
}

// gitTimestampEnvs returns the list of environment variables needed to override
// git's author and commit timestamps when creating new commits.
func gitTimestampEnvs(timestamp int) []string {
	return []string{
		fmt.Sprintf("GIT_AUTHOR_DATE=%d", timestamp),
		fmt.Sprintf("GIT_COMMITTER_DATE=%d", timestamp),
	}
}

// commandsForPullRefs returns the list of commands needed to fetch and
// merge any pull refs as well as submodules. These commands should be run only
// after the commands provided by commandsForBaseRef have been run
// successfully.
// Each merge commit will be created at sequential seconds after fakeTimestamp.
// It's recommended that fakeTimestamp be set to the timestamp of the base ref.
// This enables reproducible timestamps and git tree digests every time the same
// set of base and pull refs are used.
func (g *gitCtx) commandsForPullRefs(refs kube.Refs, fakeTimestamp int) []cloneCommand {
	var commands []cloneCommand
	for _, prRef := range refs.Pulls {
		ref := fmt.Sprintf("pull/%d/head", prRef.Number)
		if prRef.Ref != "" {
			ref = prRef.Ref
		}
		commands = append(commands, g.gitCommand("fetch", g.repositoryURI, ref))
		var prCheckout string
		if prRef.SHA != "" {
			prCheckout = prRef.SHA
		} else {
			prCheckout = "FETCH_HEAD"
		}
		fakeTimestamp++
		gitMergeCommand := g.gitCommand("merge", "--no-ff", prCheckout)
		gitMergeCommand.env = append(gitMergeCommand.env, gitTimestampEnvs(fakeTimestamp)...)
		commands = append(commands, gitMergeCommand)
	}

	// unless the user specifically asks us not to, init submodules
	if !refs.SkipSubmodules {
		commands = append(commands, g.gitCommand("submodule", "update", "--init", "--recursive"))
	}

	return commands
}

type cloneCommand struct {
	dir     string
	env     []string
	command string
	args    []string
}

func (c *cloneCommand) run() (string, string, error) {
	output := bytes.Buffer{}
	cmd := exec.Command(c.command, c.args...)
	cmd.Dir = c.dir
	cmd.Env = append(cmd.Env, c.env...)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.Join(append([]string{c.command}, c.args...), " "), output.String(), err
}

func (c *cloneCommand) String() string {
	return fmt.Sprintf("PWD=%s %s %s %s", c.dir, strings.Join(c.env, " "), c.command, strings.Join(c.env, " "))
}
