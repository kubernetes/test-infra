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
	"os"
	"testing"
	"time"
)

func TestSetDefaultEnv(t *testing.T) {
	key := "KEY"
	valOne := "1"
	valTwo := "2"
	// make sure the env is not set
	err := os.Unsetenv(key)
	if err != nil {
		t.Errorf("Failed to call os.Unsetenv to prepare test: %v", err)
	}
	// assert that it is set by SetDefaultEnv
	set, err := SetDefaultEnv(key, valOne)
	if err != nil {
		t.Errorf("encountered unexpected error calling SetDefaultEnv: %v", err)
	} else if !set {
		t.Errorf("SetDefaultEnv returned false, expected true")
	}
	// assert that val2 is not set once val1 has been set
	set, err = SetDefaultEnv(key, valTwo)
	if err != nil {
		t.Errorf("encountered unexpected error calling SetDefaultEnv: %v", err)
	} else if set {
		t.Errorf("SetDefaultEnv returned true, expected false")
	}
}

func TestEnvEqual(t *testing.T) {
	keyEqualOne := "EQUAL1"
	keyEqualTwo := "EQUAL2"
	keyNotEqual := "NOT-EQUAL"
	valEqual := "1"
	valNotEqual := "3"
	// initialize env so that keyEqualOne and keyEqualTwo are set to valEqual
	// and keyNotEqual is set to valNotEqual
	err := os.Setenv(keyEqualOne, valEqual)
	if err != nil {
		t.Errorf("Failed to call os.SetEnv to prepare test: %v", err)
	}
	err = os.Setenv(keyEqualTwo, valEqual)
	if err != nil {
		t.Errorf("Failed to call os.SetEnv to prepare test: %v", err)
	}
	err = os.Setenv(keyNotEqual, valNotEqual)
	if err != nil {
		t.Errorf("Failed to call os.SetEnv to prepare test: %v", err)
	}
	// assert that the keys are equal to each other
	equal := EnvEqual(keyEqualOne, keyEqualTwo)
	if !equal {
		t.Errorf("EnvEqual returned false, expected true")
	}
	// assert that the keys are equal to themselves
	equal = EnvEqual(keyEqualOne, keyEqualOne)
	if !equal {
		t.Errorf("EnvEqual returned false, expected true")
	}
	equal = EnvEqual(keyEqualTwo, keyEqualTwo)
	if !equal {
		t.Errorf("EnvEqual returned false, expected true")
	}
	// assert that neither of the equal keys matches the third key
	equal = EnvEqual(keyEqualOne, keyNotEqual)
	if equal {
		t.Errorf("EnvEqual returned true, expected false")
	}
	// assert that neither of the equal keys maHomeEnvtches the third key
	equal = EnvEqual(keyEqualTwo, keyNotEqual)
	if equal {
		t.Errorf("EnvEqual returned true, expected false")
	}
}

func TestSetupMagicEnviroment(t *testing.T) {
	job := "some-foo-job"
	cwd, err := os.Getwd()
	if err != nil {
		t.Errorf("got an unexpected error calling os.Getwd to setup test: %v", err)
	}

	// setup a bogus job env to make sure we override it in SetupMagiEnvironment
	err = os.Setenv(JobEnv, "a-ridiculous-value-nobody-would-use")
	if err != nil {
		t.Errorf("got an unexpected error calling os.Setenv to setup test: %v", err)
	}

	// pretend to be on jenkins
	err = os.Setenv(JenkinsHomeEnv, os.Getenv(HomeEnv))
	if err != nil {
		t.Errorf("got an unexpected error calling os.Setenv to setup test: %v", err)
	}

	// make sure other keys are not set
	err = os.Unsetenv(GCEPrivKeyEnv)
	if err != nil {
		t.Errorf("got an unexpected error calling os.Unsetenv to setup test: %v", err)
	}
	err = os.Unsetenv(GCEPubKeyEnv)
	if err != nil {
		t.Errorf("got an unexpected error calling os.Unsetenv to setup test: %v", err)
	}
	err = os.Unsetenv(AWSPrivKeyEnv)
	if err != nil {
		t.Errorf("got an unexpected error calling os.Unsetenv to setup test: %v", err)
	}
	err = os.Unsetenv(AWSPubKeyEnv)
	if err != nil {
		t.Errorf("got an unexpected error calling os.Unsetenv to setup test: %v", err)
	}

	// make sure we don't get any errors and then assert the magic environment
	err = SetupMagicEnviroment(job)
	if err != nil {
		t.Errorf("got an unexpected error calling SetupMagicEnviroment: %v", err)
	}

	// assert that we set env[JobEnv] = job
	JobVal := os.Getenv(JobEnv)
	if JobVal != job {
		t.Errorf("expected os.GetEnv(%s) to be %s but got %s", JobEnv, job, JobVal)
	}
	// assert that JenkinsHomEnv != HomeEnv
	if EnvEqual(JenkinsHomeEnv, HomeEnv) {
		t.Errorf("EnvEqual(%s, %s) returned true but should be false", JenkinsHomeEnv, HomeEnv)
	}
	// assert that the ssh key envs are set
	// TODO(bentheelder): should we also compute the exact values here and compare?
	valGCEPrivKeyEnv := os.Getenv(GCEPrivKeyEnv)
	if valGCEPrivKeyEnv == "" {
		t.Errorf("Expected os.Getenv(`%s`) to not be ``", GCEPrivKeyEnv)
	}
	valGCEPubKeyEnv := os.Getenv(GCEPubKeyEnv)
	if valGCEPubKeyEnv == "" {
		t.Errorf("Expected os.Getenv(`%s`) to not be ``", GCEPubKeyEnv)
	}
	valAWSPrivKeyEnv := os.Getenv(AWSPrivKeyEnv)
	if valAWSPrivKeyEnv == "" {
		t.Errorf("Expected os.Getenv(`%s`) to not be ``", AWSPrivKeyEnv)
	}
	valAWSPubKeyEnv := os.Getenv(AWSPubKeyEnv)
	if valAWSPubKeyEnv == "" {
		t.Errorf("Expected os.Getenv(`%s`) to not be ``", AWSPubKeyEnv)
	}
	// assert that cloud sdk env is set
	valCloudSDKEnv := os.Getenv(CloudSDKEnv)
	if valCloudSDKEnv == "" {
		t.Errorf("Expected os.Getenv(`%s`) to not be ``", CloudSDKEnv)
	}
	// assert that BootstrapEnv is "yes"
	valBootstrapEnv := os.Getenv(BootstrapEnv)
	if valBootstrapEnv != "yes" {
		t.Errorf("Expected os.Getenv(`%s`) to be \"yes\" not %#v", BootstrapEnv, valBootstrapEnv)
	}
	// assert workspace env
	valWorkspaceEnv := os.Getenv(WorkspaceEnv)
	if valWorkspaceEnv != cwd {
		t.Errorf("Expected os.Getenv(`%s`) == os.Getwd()", WorkspaceEnv)
	}
}

func TestNodeName(t *testing.T) {
	// TODO(bentheelder): improve this test?
	name, err := NodeName()
	if err != nil {
		t.Errorf("Got an unexpected err running NodeName: %v", err)
	}
	valNodeNameEnv := os.Getenv(NodeNameEnv)
	if valNodeNameEnv != name {
		t.Errorf("Expected os.Getenv(`%s`) to equal %#v not %#v", NodeNameEnv, name, valNodeNameEnv)
	}
}

func TestBuildName(t *testing.T) {
	// TODO(bentheelder): improve this test?
	name, err := BuildName(time.Unix(0, 0))
	if err != nil {
		t.Errorf("Got an unexpected err running BuildName: %v", err)
	}
	if name == "" {
		t.Errorf("Expected a non-empty build name")
	}
	valBuildNumberEnv := os.Getenv(BuildNumberEnv)
	if valBuildNumberEnv != name {
		t.Errorf("Expected os.Getenv(`%s`) to equal %#v not %#v", BuildNumberEnv, name, valBuildNumberEnv)
	}
}
