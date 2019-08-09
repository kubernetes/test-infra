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

type nodeDeploy struct{}

var _ deployer = nodeDeploy{}

// TODO(krzyzacy): Move node creation/down logic into here
func (n nodeDeploy) Up() error {
	log.Print("Noop - Node Up()")
	return nil
}

func (n nodeDeploy) IsUp() error {
	log.Print("Noop - Node IsUp()")
	return nil
}

func (n nodeDeploy) DumpClusterLogs(localPath, gcsPath string) error {
	log.Printf("Noop - Node DumpClusterLogs() - %s: %s", localPath, gcsPath)
	return nil
}

func (n nodeDeploy) TestSetup() error {
	log.Print("Noop - Node TestSetup()")
	return nil
}

func (n nodeDeploy) Down() error {
	log.Print("Noop - Node Down()")
	return nil
}

func (n nodeDeploy) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (_ nodeDeploy) KubectlCommand() (*exec.Cmd, error) {
	log.Print("Noop - Node KubectlCommand()")
	return nil, nil
}
