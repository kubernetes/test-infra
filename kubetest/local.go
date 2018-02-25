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

package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	localUpTimeout = flag.Duration("local-up-timeout", 2*time.Minute, "(local only) Time limit between 'local-up-cluster.sh' and a response from the Kubernetes API.")
)

type localCluster struct {
	tempDir    string
	kubeConfig string
}

var _ deployer = localCluster{}

func newLocalCluster() *localCluster {
	tempDir, err := ioutil.TempDir("", "kubetest-local")
	if err != nil {
		log.Fatal("unable to create temp directory")
	}
	err = os.Chmod(tempDir, 0755)
	if err != nil {
		log.Fatal("unable to change temp directory permissions")
	}
	return &localCluster{
		tempDir: tempDir,
	}
}

func (n localCluster) getScript(scriptPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cwd, scriptPath)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("unable to find script %v in directory %v", scriptPath, cwd)
}

func (n localCluster) Up() error {
	script, err := n.getScript("hack/local-up-cluster.sh")
	if err != nil {
		return err
	}
	cmd := exec.Command(script)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "ENABLE_DAEMON=true")
	cmd.Env = append(cmd.Env, fmt.Sprintf("LOG_DIR=%s", n.tempDir))
	err = control.FinishRunning(cmd)
	if err != nil {
		return err
	}
	n.kubeConfig = "/var/run/kubernetes/admin.kubeconfig"
	_, err = os.Stat(n.kubeConfig)
	return err
}

func (n localCluster) IsUp() error {
	if n.kubeConfig != "" {
		err := os.Setenv("KUBECONFIG", n.kubeConfig)
		if err != nil {
			log.Fatal("unable to set KUBECONFIG environment variable")
		}
	}
	stop := time.Now().Add(*localUpTimeout)
	for {
		script, err := n.getScript("cluster/kubectl.sh")
		if err != nil {
			return err
		}
		nodes, err := kubectlGetNodes(script)
		if err != nil {
			return err
		}
		readyNodes := countReadyNodes(nodes)
		if readyNodes > 0 {
			return nil
		}
		if time.Now().After(stop) {
			break
		} else {
			time.Sleep(5 * time.Second)
		}
	}
	return errors.New("local-up-cluster.sh is not ready")
}

func (n localCluster) DumpClusterLogs(localPath, gcsPath string) error {
	cmd := exec.Command("sudo", "cp", "-r", n.tempDir, localPath)
	return control.FinishRunning(cmd)
}

func (n localCluster) TestSetup() error {
	return nil
}

func (n localCluster) Down() error {
	err := control.FinishRunning(exec.Command("bash", "-c", "docker rm -f $(docker ps -a -q)"))
	if err != nil {
		log.Printf("unable to cleanup containers in docker: %v", err)
	}
	err = control.FinishRunning(exec.Command("pkill", "hyperkube"))
	if err != nil {
		log.Printf("unable to kill hyperkube processes: %v", err)
	}
	err = control.FinishRunning(exec.Command("pkill", "etcd"))
	if err != nil {
		log.Printf("unable to kill etcd: %v", err)
	}
	return nil
}

func (n localCluster) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("GetClusterCreated not implemented in localCluster")
}
