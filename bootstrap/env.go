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
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SetDefaultEnv does os.Setenv(key, value) if key does not exist (os.LookupEnv)
// It returns true if the key was set
func SetDefaultEnv(key, value string) (bool, error) {
	_, exists := os.LookupEnv(key)
	if !exists {
		return true, os.Setenv(key, value)
	}
	return false, nil
}

// EnvEqual returns true if both keys have the same value or both keys do not
// exist. If the values are different or if one key is "" and the other
// is not set it will return false.
func EnvEqual(key1, key2 string) bool {
	val1, exists1 := os.LookupEnv(key1)
	val2, exists2 := os.LookupEnv(key2)
	return val1 == val2 && exists1 == exists2
}

// SetupMagicEnviroment sets magic environment variables scripts currently expect.
func SetupMagicEnviroment(job string) (err error) {
	home := os.Getenv(HomeEnv)
	/*
		TODO(fejta): jenkins sets these values. Consider migrating to using
					 a secret volume instead and passing the path to this volume
					into bootstrap.py as a flag.
	*/
	_, err = SetDefaultEnv(
		GCEPrivKeyEnv,
		filepath.Join(home, ".ssh/google_compute_engine"),
	)
	if err != nil {
		return err
	}
	_, err = SetDefaultEnv(
		GCEPubKeyEnv,
		filepath.Join(home, ".ssh/google_compute_engine.pub"),
	)
	if err != nil {
		return err
	}
	_, err = SetDefaultEnv(
		AWSPrivKeyEnv,
		filepath.Join(home, ".ssh/kube_aws_rsa"),
	)
	if err != nil {
		return err
	}
	_, err = SetDefaultEnv(
		AWSPubKeyEnv,
		filepath.Join(home, ".ssh/kube_aws_rsa.pub"),
	)
	if err != nil {
		return err
	}

	// TODO(bentheelder): determine if we can avoid getcwd here :-/
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	/*
		 TODO(fejta): jenkins sets WORKSPACE and pieces of our infra expect this
					  value. Consider doing something else in the future.
		 Furthermore, in the Jenkins and Prow environments, this is already set
		 to something reasonable, but using cwd will likely cause all sorts of
		 problems. Thus, only set this if we really need to.
	*/
	_, err = SetDefaultEnv(WorkspaceEnv, cwd)
	if err != nil {
		return err
	}
	/*
	 By default, Jenkins sets HOME to JENKINS_HOME, which is shared by all
	 jobs. To avoid collisions, set it to the cwd instead, but only when
	 running on Jenkins.
	*/
	if EnvEqual(HomeEnv, JenkinsHomeEnv) {
		err = os.Setenv(HomeEnv, cwd)
		if err != nil {
			return err
		}
	}
	/*
		TODO(fejta): jenkins sets JOB_ENV and pieces of our infra expect this
					 value. Consider making everything below here agnostic to the
					 job name.
	*/
	jobVal, jobSet := os.LookupEnv(JobEnv)
	if jobSet {
		log.Printf("%s=%s (overrides %s)", JobEnv, job, jobVal)
	}
	err = os.Setenv(JobEnv, job)
	if err != nil {
		return err
	}
	// TODO(fejta): Magic value to tell our test code not do upload started.json
	// TODO(fejta): delete upload-to-gcs.sh and then this value.
	err = os.Setenv(BootstrapEnv, "yes")
	if err != nil {
		return err
	}
	// This helps prevent reuse of cloudsdk configuration. It also reduces the
	// risk that running a job on a workstation corrupts the user's config.
	return os.Setenv(CloudSDKEnv, filepath.Join(cwd, ".config", "gcloud"))
}

// utility method used in NodeName and BuildName
// NOTE: this will not produce the same value as hash(str) in python but
// it does have similar characteristics
func hash(s string) uint32 {
	hasher := fnv.New32a()
	hasher.Write([]byte(s))
	return hasher.Sum32()
}

// NodeName returns the name of the node the build is running on
// and sets os.Setenv(NodeNameEnv, res) if not already set
func NodeName() (string, error) {
	// TODO(fejta): jenkins sets the node name and our infra expect this value.
	// TODO(fejta): Consider doing something different here.
	_, exists := os.LookupEnv(NodeNameEnv)
	if !exists {
		hostname, err := os.Hostname()
		if err != nil {
			return "", err
		}
		name := strings.Join(strings.Split(hostname, ".")[1:], "")
		err = os.Setenv(NodeNameEnv, name)
		if err != nil {
			return "", err
		}
	}
	return os.Getenv(NodeNameEnv), nil
}

// BuildName returns the name of the ID/name for the current build
// and sets os.Setenv(BuildNumberEnv, res) if not already set
func BuildName(started time.Time) (string, error) {
	/*
		TODO(fejta): right now jenkins sets the BUILD_NUMBER and does this
					 in an environment variable. Consider migrating this to a
					 bootstrap.py flag
	*/
	_, exists := os.LookupEnv(BuildNumberEnv)
	if !exists {
		// Automatically generate a build number if none is set
		nodeName, err := NodeName()
		if err != nil {
			return "", err
		}
		uniq := fmt.Sprintf("%x-%d", hash(nodeName), os.Getpid())
		autogen := started.Format("20060102-150400-") + uniq
		err = os.Setenv(BuildNumberEnv, autogen)
		if err != nil {
			return "", err
		}
	}
	return os.Getenv(BuildNumberEnv), nil
}
