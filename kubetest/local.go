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
	"net"
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
	cmd := exec.Command("cp", "-r", n.tempDir, localPath)
	return control.FinishRunning(cmd)
}

func (n localCluster) TestSetup() error {
	return nil
}

func (n localCluster) Down() error {
	processes := []string{
		"cloud-controller-manager",
		"kube-controller-manager",
		"kube-proxy",
		"kube-scheduler",
		"kube-apiserver",
		"kubelet",
	}

	// make sure all containers are removed
	if err := control.FinishRunning(exec.Command("sh", "-c", `docker ps -aq | xargs docker rm -fv`)); err != nil {
		log.Printf("unable to cleanup containers in docker: %v", err)
	}

	for _, p := range processes {
		// -f is required to match against the complete command line
		// (/proc/pid/cmdline), otherwise process name longer than 15
		// characters cannot be matched, see https://linux.die.net/man/1/pkill.
		if err := control.FinishRunning(exec.Command("pkill", "-f", p)); err != nil {
			log.Printf("unable to kill kubernetes process %q: %v", p, err)
		}
	}
	if err := control.FinishRunning(exec.Command("pkill", "etcd")); err != nil {
		log.Printf("unable to kill etcd: %v", err)
	}
	return nil
}

func (n localCluster) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("GetClusterCreated not implemented in localCluster")
}

func (_ localCluster) KubectlCommand() (*exec.Cmd, error) { return nil, nil }
