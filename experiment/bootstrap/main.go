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

// bootstrap implements a drop-in-replacement for jenkins/bootstrap.py
package main

import (
	"fmt"
	"log"
	"os"
)

// Constant Keys for known environment variables and URLs
const (
	BuildNumberEnv         string = "BUILD_NUMBER"
	BootstrapEnv           string = "BOOTSTRAP_MIGRATION"
	CloudSDKEnv            string = "CLOUDSDK_CONFIG"
	GCEPrivKeyEnv          string = "JENKINS_GCE_SSH_PRIVATE_KEY_FILE"
	GCEPubKeyEnv           string = "JENKINS_GCE_SSH_PUBLIC_KEY_FILE"
	AWSPrivKeyEnv          string = "JENKINS_AWS_SSH_PRIVATE_KEY_FILE"
	AWSPubKeyEnv           string = "JENKINS_AWS_SSH_PUBLIC_KEY_FILE"
	GubernatorBaseBuildURL string = "https://gubernator.k8s.io/build/"
	HomeEnv                string = "HOME"
	JenkinsHomeEnv         string = "JENKINS_HOME"
	JobEnv                 string = "JOB_NAME"
	NodeNameEnv            string = "NODE_NAME"
	ServiceAccountEnv      string = "GOOGLE_APPLICATION_CREDENTIALS"
	WorkspaceEnv           string = "WORKSPACE"
	GCSArtifactsEnv        string = "GCS_ARTIFACTS_DIR"
)

// Bootstrap is the "real main" for bootstrap, after argument parsing
func Bootstrap(args *Args) error {
	repos, err := ParseRepos(args.Repo)
	if err != nil {
		return err
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get CWD! %v", err)
	}

	build := "fake" // TODO: port computing this value

	var paths *Paths
	if args.Upload != "" {
		if repos.Main().Pull != "" {
			paths, err = PRPaths(args.Upload, repos, args.Job, build)
			if err != nil {
				return err
			}
		} else {
			paths = CIPaths(args.Upload, args.Job, build)
		}
		// TODO(fejta): Replace env var below with a flag eventually.
		os.Setenv(GCSArtifactsEnv, paths.Artifacts)
	}

	// TODO(bentheelder): mimic the rest of bootstrap.py here ¯\_(ツ)_/¯
	// Printing these so that it compiles ¯\_(ツ)_/¯
	fmt.Println(originalCWD)
	fmt.Println(paths)
	return nil
}

func main() {
	args, err := ParseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("error parsing args: %v", err)
	}
	err = Bootstrap(args)
	if err != nil {
		log.Fatalf("bootstrap error: %v", err)
	}
}
