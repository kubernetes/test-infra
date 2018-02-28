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
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
)

func Run(refs kube.Refs, dir, gitUserName, gitUserEmail string) Record {
	logrus.WithFields(logrus.Fields{"refs": refs}).Infof("Cloning refs")
	repositoryURL := fmt.Sprintf("https://github.com/%s/%s.git", refs.Org, refs.Repo)
	cloneDir := fmt.Sprintf("%s/src/github.com/%s/%s", dir, refs.Org, refs.Repo)
	record := Record{Refs: refs}

	commands := []cloneCommand{
		func() (string, string, error) {
			return fmt.Sprintf("os.MkdirAll(%s, 0755)", cloneDir), "", os.MkdirAll(cloneDir, 0755)
		},
	}

	commands = append(commands, shellCloneCommand(cloneDir, "git", "init"))
	if gitUserName != "" {
		commands = append(commands, shellCloneCommand(cloneDir, "git", "config", "user.name", gitUserName))
	}
	if gitUserEmail != "" {
		commands = append(commands, shellCloneCommand(cloneDir, "git", "config", "user.email", gitUserEmail))
	}
	commands = append(commands, shellCloneCommand(cloneDir, "git", "fetch", repositoryURL, refs.BaseRef))

	var checkout string
	if refs.BaseSHA != "" {
		checkout = refs.BaseSHA
	} else {
		checkout = "FETCH_HEAD"
	}
	commands = append(commands, shellCloneCommand(cloneDir, "git", "checkout", checkout))

	for _, prRef := range refs.Pulls {
		commands = append(commands, shellCloneCommand(cloneDir, "git", "fetch", repositoryURL, fmt.Sprintf("pull/%d/head", prRef.Number)))
		var prCheckout string
		if prRef.SHA != "" {
			prCheckout = prRef.SHA
		} else {
			prCheckout = "FETCH_HEAD"
		}
		commands = append(commands, shellCloneCommand(cloneDir, "git", "merge", prCheckout))
	}

	for _, command := range commands {
		formattedCommand, output, err := command()
		logrus.WithFields(logrus.Fields{"command": formattedCommand, "output": output, "error": err}).Infof("Ran clone command")
		message := ""
		if err != nil {
			message = err.Error()
			record.Failed = true
		}
		record.Commands = append(record.Commands, Command{Command: formattedCommand, Output: output, Error: message})
		if err != nil {
			break
		}
	}

	return record
}

type cloneCommand func() (string, string, error)

func shellCloneCommand(dir, command string, args ...string) cloneCommand {
	output := bytes.Buffer{}
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Stdout = &output
	cmd.Stderr = &output

	return func() (string, string, error) {
		err := cmd.Run()
		return strings.Join(append([]string{command}, args...), " "), output.String(), err
	}
}
