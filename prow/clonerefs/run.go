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

package clonerefs

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

var cloneFunc = clone.Run

func readOauthToken(path string) (string, clone.Command, error) {
	data, err := ioutil.ReadFile(path)
	cmd := clone.Command{
		Command: fmt.Sprintf("golang: read %q", path),
		Output:  "redacted",
	}
	if err != nil {
		cmd.Error = err.Error()
		return "", cmd, err
	}
	return strings.TrimSpace(string(data)), cmd, nil
}

func (o *Options) createRecords() []clone.Record {
	var rec clone.Record
	var env []string
	if len(o.KeyFiles) > 0 {
		var err error
		var cmds []clone.Command
		env, cmds, err = addSSHKeys(o.KeyFiles)
		rec.Commands = append(rec.Commands, cmds...)
		if err != nil {
			logrus.WithError(err).Error("Failed to add SSH keys.")
			rec.Failed = true
			return []clone.Record{rec}
		}
	}
	if len(o.HostFingerprints) > 0 {
		envVar, cmds, err := addHostFingerprints(o.HostFingerprints)
		rec.Commands = append(rec.Commands, cmds...)
		if err != nil {
			logrus.WithError(err).Error("failed to add host fingerprints")
			rec.Failed = true
			return []clone.Record{rec}
		}
		env = append(env, envVar)
	}

	var oauthToken string
	if o.OauthTokenFile != "" {
		token, cmd, err := readOauthToken(o.OauthTokenFile)
		rec.Commands = append(rec.Commands, cmd)
		if err != nil {
			logrus.WithError(err).Error("Failed to read oauth key file.")
			rec.Failed = true
			return []clone.Record{rec}
		}
		oauthToken = token
	}

	if p := needsGlobalCookiePath(o.CookiePath, o.GitRefs...); p != "" {
		cmd, err := configureGlobalCookiefile(p)
		rec.Commands = append(rec.Commands, cmd)
		if err != nil {
			logrus.WithError(err).WithField("path", p).Error("Failed to configure global cookiefile")
			rec.Failed = true
			return []clone.Record{rec}
		}
	}

	var numWorkers int
	if o.MaxParallelWorkers != 0 {
		numWorkers = o.MaxParallelWorkers
	} else {
		numWorkers = len(o.GitRefs)
	}

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	input := make(chan prowapi.Refs)
	output := make(chan clone.Record, len(o.GitRefs))
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for ref := range input {
				output <- cloneFunc(ref, o.SrcRoot, o.GitUserName, o.GitUserEmail, o.CookiePath, env, oauthToken)
			}
		}()
	}

	for _, ref := range o.GitRefs {
		input <- ref
	}

	close(input)
	wg.Wait()
	close(output)

	results := []clone.Record{rec}
	for record := range output {
		results = append(results, record)
	}
	return results
}

// Run clones the configured refs
func (o Options) Run() error {
	results := o.createRecords()
	logData, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal clone records: %v", err)
	}

	if err := ioutil.WriteFile(o.Log, logData, 0755); err != nil {
		return fmt.Errorf("write clone records: %v", err)
	}

	var failed int
	for _, record := range results {
		if record.Failed {
			failed++
		}
	}

	if o.Fail && failed > 0 {
		return fmt.Errorf("%d clone records failed", failed)
	}

	return nil
}

func needsGlobalCookiePath(cookieFile string, refs ...prowapi.Refs) string {
	if cookieFile == "" || len(refs) == 0 {
		return ""
	}

	for _, r := range refs {
		if !r.SkipSubmodules {
			return cookieFile
		}
	}
	return ""
}

// configureGlobalCookiefile ensures git authenticates submodules correctly.
//
// Since this is a global setting, we do it once and before running parallel clones.
func configureGlobalCookiefile(cookiePath string) (clone.Command, error) {
	out, err := exec.Command("git", "config", "--global", "http.cookiefile", cookiePath).CombinedOutput()
	cmd := clone.Command{
		Command: fmt.Sprintf("git config --global http.cookiefile %q", cookiePath),
		Output:  string(out),
	}
	if err != nil {
		cmd.Error = err.Error()
	}
	return cmd, err
}

func addHostFingerprints(fingerprints []string) (string, []clone.Command, error) {
	// let's try to create the tmp dir if it doesn't exist
	var cmds []clone.Command
	sshDir := "/tmp"
	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		err := os.MkdirAll(sshDir, 0755)
		cmd := clone.Command{
			Command: fmt.Sprintf("golang: create %q", sshDir),
		}
		if err != nil {
			cmd.Error = err.Error()
		}
		cmds = append(cmds, cmd)
		if err != nil {
			return "", cmds, fmt.Errorf("create sshDir %s: %v", sshDir, err)
		}
	}

	knownHostsFile := filepath.Join(sshDir, "known_hosts")
	f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	cmd := clone.Command{
		Command: fmt.Sprintf("golang: append %q", knownHostsFile),
	}
	if err != nil {
		cmd.Error = err.Error()
		cmds = append(cmds, cmd)
		return "", cmds, fmt.Errorf("append %s: %v", knownHostsFile, err)
	}

	if _, err := f.Write([]byte(strings.Join(fingerprints, "\n"))); err != nil {
		cmd.Error = err.Error()
		cmds = append(cmds, cmd)
		return "", cmds, fmt.Errorf("write fingerprints to %s: %v", knownHostsFile, err)
	}
	if err := f.Close(); err != nil {
		cmd.Error = err.Error()
		cmds = append(cmds, cmd)
		return "", cmds, fmt.Errorf("close %s: %v", knownHostsFile, err)
	}
	cmds = append(cmds, cmd)
	logrus.Infof("Updated known_hosts in file: %s", knownHostsFile)

	ssh, err := exec.LookPath("ssh")
	cmd = clone.Command{
		Command: "golang: lookup ssh path",
	}

	if err != nil {
		cmd.Error = err.Error()
		cmds = append(cmds, cmd)
		return "", cmds, fmt.Errorf("lookup ssh path: %v", err)
	}
	cmds = append(cmds, cmd)
	return fmt.Sprintf("GIT_SSH_COMMAND=%s -o UserKnownHostsFile=%s", ssh, knownHostsFile), cmds, nil
}

// addSSHKeys will start the ssh-agent and add all the specified
// keys, returning the ssh-agent environment variables for reuse
func addSSHKeys(paths []string) ([]string, []clone.Command, error) {
	var cmds []clone.Command
	vars, err := exec.Command("ssh-agent").CombinedOutput()
	cmd := clone.Command{
		Command: "ssh-agent",
		Output:  string(vars),
	}
	if err != nil {
		cmd.Error = err.Error()
	}
	cmds = append(cmds, cmd)
	if err != nil {
		return []string{}, cmds, fmt.Errorf("start ssh-agent: %v", err)
	}
	logrus.Info("Started SSH agent")
	// ssh-agent will output three lines of text, in the form:
	// SSH_AUTH_SOCK=xxx; export SSH_AUTH_SOCK;
	// SSH_AGENT_PID=xxx; export SSH_AGENT_PID;
	// echo Agent pid xxx;
	// We need to parse out the environment variables from that.
	parts := strings.Split(string(vars), ";")
	env := []string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2])}
	for _, keyPath := range paths {
		// we can be given literal paths to keys or paths to dirs
		// that are mounted from a secret, so we need to check which
		// we have
		if err := filepath.Walk(keyPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(info.Name(), "..") {
				// kubernetes volumes also include files we
				// should not look be looking into for keys
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}

			cmd := exec.Command("ssh-add", path)
			cmd.Env = append(cmd.Env, env...)
			output, err := cmd.CombinedOutput()
			cloneCmd := clone.Command{
				Command: fmt.Sprintf("ssh-add %q", path),
				Output:  string(output),
			}
			if err != nil {
				cloneCmd.Error = err.Error()
			}
			cmds = append(cmds, cloneCmd)
			if err != nil {
				return fmt.Errorf("add ssh key at %s: %v: %s", path, err, output)
			}
			logrus.Infof("Added SSH key at %s", path)
			return nil
		}); err != nil {
			return env, cmds, fmt.Errorf("walking path %q: %v", keyPath, err)
		}
	}
	return env, cmds, nil
}
