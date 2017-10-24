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
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// TODO(bentheelder): improve usage documentation
// TODO(bentheelder): how can we support DOCKER_EXTRA in a sane fashion?

// Defaults
const (
	// DefaultImageName is the default docker image to run
	DefaultImageName = "gcr.io/k8s-testimages/planter"
	// DefaultTag is the default docker image tag to run
	// NOTE: keep in sync with planter.sh / Makefile !
	DefaultTag = "0.7.0-1"
)

// Env keys
const (
	// ImageEnv is the environment variable used to override the image name
	ImageEnv = "IMAGE_NAME"
	// TagEnv is the environment variable used to override the image tag
	TagEnv = "TAG"
	// If set, only echo the docker command that would have been run
	DryRunEnv = "DRY_RUN"
)

func getenvDefault(key, defaultValue string) string {
	value, isSet := os.LookupEnv(key)
	if isSet {
		return value
	}
	return defaultValue
}

func getWorkspaceDir() string {
	// TODO(bentheelder): maybe we should just *actually* search for WORKSPACE?
	// try to get the current git dir
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.Trim(string(out), "\n")
}

func main() {
	imageName := getenvDefault(ImageEnv, DefaultImageName)
	tag := getenvDefault(TagEnv, DefaultTag)
	image := imageName + ":" + tag

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get working dir! %s\b", err)
		os.Exit(-1)
	}

	// run our docker image as the host user with bazel cache and current repo dir
	workspaceDir := getWorkspaceDir()
	// fallback to PWD
	// TODO(bentheelder): maybe someone has a copy of the repo without .git?
	// in this case we could probably search for the closest WORKSPACE to $PWD
	if workspaceDir == "" {
		workspaceDir = wd
	}

	u, err := user.Current()
	if err != nil {
		fmt.Printf("Failed to get the current user! %s\n", err)
		os.Exit(-1)
	}

	// TODO(bentheelder): how do we map these volumes etc in on Windows??
	// construct the docker command to run the planter image
	cmdArr := []string{
		"docker", "run",
		"--rm", // remove container after running
		// volumes
		"-v", workspaceDir + ":" + workspaceDir, // mount WORKSPACE directory
		"-v", u.HomeDir + ":" + u.HomeDir, // mount the user's home dir for the bazel cache
		"--tmpfs", "/tmp:exec,mode=777", // make sure bazel can use the tmpfs in /tmp
		"-w", wd, // set workdir to $CWD
		// run as the current user / pass envs to the entrypoint which fakes /etc/passwd
		"--user", u.Uid,
		"-e", "USER=" + u.Username,
		"-e", "GID=" + u.Gid,
		"-e", "UID=" + u.Uid,
		"-e", "HOME=" + u.HomeDir,
		// specify the image
		image,
	}
	// followed by the command line args which will run inside the image
	cmdArr = append(cmdArr, os.Args[1:]...)

	// print out the constructed command if in dry mode, otherwise run
	if _, isSet := os.LookupEnv(DryRunEnv); isSet {
		fmt.Println(strings.Join(cmdArr, " "))
	} else {
		// run in current shell
		cmd := exec.Command(cmdArr[0], cmdArr[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			fmt.Printf("Failed to run command: %v", err)
			os.Exit(-1)
		}
	}
}
