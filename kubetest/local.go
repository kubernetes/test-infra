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
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var (
	localUpTimeout = flag.Duration("local-up-timeout", 2*time.Minute, "(local only) Time limit between 'local-up-cluster.sh' and a response from the Kubernetes API.")
)

func removeAllContainers(cli *client.Client) {
	// list all containers
	listOptions := types.ContainerListOptions{
		Quiet: true,
		All:   true,
	}
	containers, err := cli.ContainerList(context.Background(), listOptions)
	if err != nil {
		log.Printf("Failed to list containers: %v\n", err)
		return
	}

	// reverse sort by Creation time so we delete newest containers first
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Created > containers[j].Created
	})

	// stop then remove (which implicitly kills) each container
	duration := time.Second * 1
	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}
	for _, container := range containers {
		log.Printf("Stopping container: %v %s with ID: %s\n",
			container.Names, container.Image, container.ID)
		err = cli.ContainerStop(context.Background(), container.ID, &duration)
		if err != nil {
			log.Printf("Error stopping container: %v\n", err)
		}

		log.Printf("Removing container: %v %s with ID: %s\n",
			container.Names, container.Image, container.ID)
		err = cli.ContainerRemove(context.Background(), container.ID, removeOptions)
		if err != nil {
			log.Printf("Error removing container: %v\n", err)
		}
	}
}

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

	// Needed for at least one conformance e2e test. Please see issue #59978
	cmd.Env = append(cmd.Env, "ALLOW_PRIVILEGED=true")

	// when we are running in a DIND scenario, we should use the ip address of
	// the docker0 network interface, This ensures that when the pods come up
	// the health checks (for example for kubedns) succeed. If there is no
	// docker0, just use the defaults in local-up-cluster.sh
	dockerIP := ""
	docker0, err := net.InterfaceByName("docker0")
	if err == nil {
		addresses, err := docker0.Addrs()
		if err == nil {
			for _, address := range addresses {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						dockerIP = ipnet.IP.String()
						break
					}
				}
			}
		} else {
			log.Printf("unable to get addresses from docker0 interface : %v", err)
		}
	} else {
		log.Printf("unable to find docker0 interface : %v", err)
	}
	if dockerIP != "" {
		log.Printf("using %v for API_HOST_IP, HOSTNAME_OVERRIDE, KUBELET_HOST", dockerIP)
		cmd.Env = append(cmd.Env, fmt.Sprintf("API_HOST_IP=%s", dockerIP))
		cmd.Env = append(cmd.Env, fmt.Sprintf("HOSTNAME_OVERRIDE=%s", dockerIP))
		cmd.Env = append(cmd.Env, fmt.Sprintf("KUBELET_HOST=%s", dockerIP))
	} else {
		log.Println("using local-up-cluster.sh's defaults for API_HOST_IP, HOSTNAME_OVERRIDE, KUBELET_HOST")
	}

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
		if err := os.Setenv("KUBECONFIG", n.kubeConfig); err != nil {
			return err
		}
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_PROVIDER", "local"); err != nil {
		return err
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
	// create docker client
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Printf("Docker containers cleanup, unable to create Docker client: %v", err)
	}
	// make sure all containers are removed
	removeAllContainers(cli)
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

func (_ localCluster) KubectlCommand() (*exec.Cmd, error) { return nil, nil }
