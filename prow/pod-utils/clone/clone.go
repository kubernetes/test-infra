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
	"net/url"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/logrusutil"
)

type runnable interface {
	run() (string, string, error)
}

// Run clones the refs under the prescribed directory and optionally
// configures the git username and email in the repository as well.
func Run(refs prowapi.Refs, dir, gitUserName, gitUserEmail, cookiePath string, env []string, oauthToken string) Record {
	if len(oauthToken) > 0 {
		logrus.SetFormatter(logrusutil.NewCensoringFormatter(logrus.StandardLogger().Formatter, func() sets.String {
			return sets.NewString(oauthToken)
		}))
	}
	logrus.WithFields(logrus.Fields{"refs": refs}).Info("Cloning refs")
	record := Record{Refs: refs}

	// This function runs the provided commands in order, logging them as they run,
	// aborting early and returning if any command fails.
	runCommands := func(commands []runnable) error {
		for _, command := range commands {
			formattedCommand, output, err := command.run()
			log := logrus.WithFields(logrus.Fields{"command": formattedCommand, "output": output})
			if err != nil {
				log = log.WithField("error", err)
			}
			log.Info("Ran command")
			message := ""
			if err != nil {
				message = err.Error()
				record.Failed = true
			}
			record.Commands = append(record.Commands, Command{Command: censorToken(formattedCommand, oauthToken), Output: censorToken(output, oauthToken), Error: censorToken(message, oauthToken)})
			if err != nil {
				return err
			}
		}
		return nil
	}

	g := gitCtxForRefs(refs, dir, env, oauthToken)
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

	finalSHA, err := g.gitRevParse()
	if err != nil {
		logrus.WithError(err).Warnf("Cannot resolve finalSHA for ref %#v", refs)
	} else {
		record.FinalSHA = finalSHA
	}

	return record
}

func censorToken(msg, token string) string {
	if token == "" {
		return msg
	}
	censored := bytes.ReplaceAll([]byte(msg), []byte(token), []byte("CENSORED"))
	return string(censored)
}

// PathForRefs determines the full path to where
// refs should be cloned
func PathForRefs(baseDir string, refs prowapi.Refs) string {
	var clonePath string
	if refs.PathAlias != "" {
		clonePath = refs.PathAlias
	} else {
		clonePath = fmt.Sprintf("github.com/%s/%s", refs.Org, refs.Repo)
	}
	return path.Join(baseDir, "src", clonePath)
}

// gitCtx collects a few common values needed for all git commands.
type gitCtx struct {
	cloneDir      string
	env           []string
	repositoryURI string
}

// gitCtxForRefs creates a gitCtx based on the provide refs and baseDir.
func gitCtxForRefs(refs prowapi.Refs, baseDir string, env []string, oauthToken string) gitCtx {
	g := gitCtx{
		cloneDir:      PathForRefs(baseDir, refs),
		env:           env,
		repositoryURI: fmt.Sprintf("https://github.com/%s/%s.git", refs.Org, refs.Repo),
	}
	if refs.CloneURI != "" {
		g.repositoryURI = refs.CloneURI
	}

	if len(oauthToken) > 0 {
		u, _ := url.Parse(g.repositoryURI)
		u.User = url.UserPassword(oauthToken, "x-oauth-basic")
		g.repositoryURI = u.String()
	}

	return g
}

func (g *gitCtx) gitCommand(args ...string) cloneCommand {
	return cloneCommand{dir: g.cloneDir, env: g.env, command: "git", args: args}
}

var (
	fetchRetries = []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		2 * time.Second,
	}
)

func (g *gitCtx) gitFetch(fetchArgs ...string) retryCommand {
	args := []string{"fetch"}
	args = append(args, fetchArgs...)

	return retryCommand{
		runnable: g.gitCommand(args...),
		retries:  fetchRetries,
	}
}

// commandsForBaseRef returns the list of commands needed to initialize and
// configure a local git directory, as well as fetch and check out the provided
// base ref.
func (g *gitCtx) commandsForBaseRef(refs prowapi.Refs, gitUserName, gitUserEmail, cookiePath string) []runnable {
	var commands []runnable
	commands = append(commands, cloneCommand{dir: "/", env: g.env, command: "mkdir", args: []string{"-p", g.cloneDir}})

	commands = append(commands, g.gitCommand("init"))
	if gitUserName != "" {
		commands = append(commands, g.gitCommand("config", "user.name", gitUserName))
	}
	if gitUserEmail != "" {
		commands = append(commands, g.gitCommand("config", "user.email", gitUserEmail))
	}
	if cookiePath != "" && refs.SkipSubmodules {
		commands = append(commands, g.gitCommand("config", "http.cookiefile", cookiePath))
	}

	if refs.CloneDepth > 0 {
		commands = append(commands, g.gitFetch(g.repositoryURI, "--tags", "--prune", "--depth", strconv.Itoa(refs.CloneDepth)))
		commands = append(commands, g.gitFetch("--depth", strconv.Itoa(refs.CloneDepth), g.repositoryURI, refs.BaseRef))
	} else {
		commands = append(commands, g.gitFetch(g.repositoryURI, "--tags", "--prune"))
		commands = append(commands, g.gitFetch(g.repositoryURI, refs.BaseRef))
	}
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
	timestamp, convErr := strconv.Atoi(strings.TrimSpace(string(gitOutput)))
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

// gitRevParse returns current commit from HEAD in a git tree
func (g *gitCtx) gitRevParse() (string, error) {
	gitRevParseCommand := g.gitCommand("rev-parse", "HEAD")
	_, commit, err := gitRevParseCommand.run()
	if err != nil {
		logrus.WithError(err).Error("git rev-parse HEAD failed!")
		return "", err
	}
	return strings.TrimSpace(commit), nil
}

// commandsForPullRefs returns the list of commands needed to fetch and
// merge any pull refs as well as submodules. These commands should be run only
// after the commands provided by commandsForBaseRef have been run
// successfully.
// Each merge commit will be created at sequential seconds after fakeTimestamp.
// It's recommended that fakeTimestamp be set to the timestamp of the base ref.
// This enables reproducible timestamps and git tree digests every time the same
// set of base and pull refs are used.
func (g *gitCtx) commandsForPullRefs(refs prowapi.Refs, fakeTimestamp int) []runnable {
	var commands []runnable
	for _, prRef := range refs.Pulls {
		ref := fmt.Sprintf("pull/%d/head", prRef.Number)
		if prRef.Ref != "" {
			ref = prRef.Ref
		}
		commands = append(commands, g.gitFetch(g.repositoryURI, ref))
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

type retryCommand struct {
	runnable
	retries []time.Duration
}

func (rc retryCommand) run() (string, string, error) {
	cmd, out, err := rc.runnable.run()
	if err == nil {
		return cmd, out, err
	}
	for _, dur := range rc.retries {
		logrus.WithError(err).WithFields(logrus.Fields{
			"sleep":   dur,
			"command": cmd,
		}).Info("Retrying after sleep")
		time.Sleep(dur)
		cmd, out, err = rc.runnable.run()
		if err == nil {
			break
		}
	}
	return cmd, out, err
}

type cloneCommand struct {
	dir     string
	env     []string
	command string
	args    []string
}

func (c cloneCommand) run() (string, string, error) {
	var output bytes.Buffer
	cmd := exec.Command(c.command, c.args...)
	cmd.Dir = c.dir
	cmd.Env = append(cmd.Env, c.env...)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.Join(append([]string{c.command}, c.args...), " "), output.String(), err
}

func (c cloneCommand) String() string {
	return fmt.Sprintf("PWD=%s %s %s %s", c.dir, strings.Join(c.env, " "), c.command, strings.Join(c.env, " "))
}
