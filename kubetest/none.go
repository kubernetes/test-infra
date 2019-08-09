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
	"errors"
	"log"
	"os/exec"
	"time"
)

type noneDeploy struct{}

var _ deployer = noneDeploy{}

func (n noneDeploy) Up() error {
	log.Print("Noop Up()")
	return nil
}

func (n noneDeploy) IsUp() error {
	log.Print("Noop IsUp()")
	return nil
}

func (n noneDeploy) DumpClusterLogs(localPath, gcsPath string) error {
	return defaultDumpClusterLogs(localPath, gcsPath)
}

func (n noneDeploy) TestSetup() error {
	log.Print("Noop TestSetup()")
	return nil
}

func (n noneDeploy) Down() error {
	log.Print("Noop Down()")
	return nil
}

func (n noneDeploy) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (_ noneDeploy) KubectlCommand() (*exec.Cmd, error) {
	log.Print("Noop KubectlCommand()")
	return nil, nil
}
