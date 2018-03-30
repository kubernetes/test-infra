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

// Package dind implements dind-specific kubetest code.
package dind

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"golang.org/x/net/context"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"
)

// Builder is capable of building the appropriate artifacts for the dind deployment.
type Builder struct {
	KubeRoot string
	ToolRoot string
	control  *process.Control
}

// NewBuilder returns an object capable of building the required k8s artifacts for dind.
func NewBuilder(kubeRoot, toolRoot string, control *process.Control) *Builder {
	return &Builder{
		KubeRoot: kubeRoot,
		ToolRoot: toolRoot,
		control:  control,
	}
}

// Build creates the k8s artifacts for dind.
func (b *Builder) Build() error {
	// Set environment variables for the build, and go to our make dir. Then put everything back.
	k, err := util.PushEnv("KUBE_ROOT", b.KubeRoot)
	if err != nil {
		return err
	}
	defer k()
	t, err := util.PushEnv("TOOL_ROOT", b.ToolRoot)
	if err != nil {
		return err
	}
	defer t()
	d, err := util.Pushd(b.ToolRoot)
	if err != nil {
		return err
	}
	defer d()

	// Run the build command.
	log.Printf("KUBE_ROOT: %s", os.Getenv("KUBE_ROOT"))
	log.Printf("TOOL_ROOT: %s", os.Getenv("TOOL_ROOT"))
	cmd := exec.Command("make", "-C", b.ToolRoot, "cluster")
	return b.control.FinishRunning(cmd)
}

// Tester is capable of running tests against a dind cluster.
type Tester struct {
	kubecfg   string
	ginkgo    string
	e2etest   string
	control   *process.Control
	apiserver *kubernetes.Clientset
	testArgs  *string
	reportdir string
}

// NewTester returns an object that knows how to test the cluster it deployed.
//TODO(Q-Lee): the deployer interfact should have a NewTester or Test method.
func (d *DindDeployer) NewTester() (*Tester, error) {
	// Find the ginkgo and e2e.test artifacts we need. We'll cheat for now, and pull them from a known path.
	// We only support dind from linux_amd64 anyway.
	ginkgo := util.K8s("kubernetes", "bazel-bin", "vendor", "github.com", "onsi", "ginkgo", "ginkgo", "linux_amd64_stripped", "ginkgo")
	if _, err := os.Stat(ginkgo); err != nil {
		return nil, fmt.Errorf("ginko isn't available at %s: %v", ginkgo, err)
	}
	e2etest := util.K8s("kubernetes", "bazel-bin", "test", "e2e", "e2e.test")
	if _, err := os.Stat(e2etest); err != nil {
		return nil, fmt.Errorf("e2e.test isn't available at %s: %v", e2etest, err)
	}
	// Make a tmpdir for the test report.
	// TODO(Q-Lee): perhaps there should be one tmpdir per cluster container, and all else can be subdirs?
	tmpdir, err := ioutil.TempDir("/tmp", "dind-k8s-report-dir-")
	if err != nil {
		return nil, err
	}

	return &Tester{
		kubecfg:   d.RealKubecfg,
		control:   d.control,
		apiserver: d.apiserver,
		testArgs:  d.testArgs,
		e2etest:   e2etest,
		ginkgo:    ginkgo,
		reportdir: tmpdir,
	}, nil
}

// Test just execs ginkgo. This will take more parameters in the future.
func (t *Tester) Test() error {
	skipRegex := "--skip=\"(Feature)|(NFS)|(StatefulSet)\""
	focusRegex := "--focus=\".*Conformance.*\""
	args := []string{"--seed=1436380640", "--nodes=10", skipRegex, focusRegex, t.e2etest,
		"--", "--kubeconfig", t.kubecfg, "--ginkgo.flakeAttempts=2", "--num-nodes=4", "--systemd-services=docker,kubelet",
		"--report-dir", t.reportdir}
	args = append(args, strings.Fields(*t.testArgs)...)
	cmd := exec.Command(t.ginkgo, args...)
	return t.control.FinishRunning(cmd)
}

type DindDeployer struct {
	image       string
	kubecfg     string
	containerID string
	tmpdir      string
	RealKubecfg string
	testArgs    *string
	docker      *client.Client
	control     *process.Control
	apiserver   *kubernetes.Clientset
}

// New returns a new DindDeployer.
func NewDeployer(kubecfg, image string, testArgs *string, control *process.Control) (*DindDeployer, error) {
	docker, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	// Make a tmpdir for kubecfg.
	tmpdir, err := ioutil.TempDir("/tmp", "dind-k8s-")
	if err != nil {
		return nil, err
	}
	log.Printf("Created tmpdir %s for kubecfg", tmpdir)

	if kubecfg == "" {
		kubecfg = tmpdir + "/admin.conf"
	}
	return &DindDeployer{
		image:    image,
		kubecfg:  kubecfg,
		tmpdir:   tmpdir,
		docker:   docker,
		control:  control,
		testArgs: testArgs,
	}, nil
}

func (d *DindDeployer) ensureImage() error {
	// Once Kubernetes is built, we should grab the version for dind.
	if d.image == "" {
		tag, err := GetDockerVersion()
		if err != nil {
			return err
		}
		d.image = "gcr.io/google-containers/dind-cluster-amd64:" + tag
	}
	return nil

}

// Up synchronously starts a cluster, or times out.
func (d *DindDeployer) Up() error {
	if err := d.ensureImage(); err != nil {
		return err
	}

	ctx := context.Background()

	resp, err := d.docker.ContainerCreate(ctx, &container.Config{
		Image:      d.image,
		Entrypoint: []string{"/init-wrapper.sh"},
	}, &container.HostConfig{
		CapAdd: []string{"SYS_ADMIN"},
		Mounts: []mount.Mount{
			{
				Source: "/lib/modules",
				Target: "/lib/modules",
				Type:   "bind",
				BindOptions: &mount.BindOptions{
					Propagation: mount.PropagationRShared,
				},
			},
			{
				Source: "/sys/fs/cgroup",
				Target: "/sys/fs/cgroup",
				Type:   "bind",
				BindOptions: &mount.BindOptions{
					Propagation: mount.PropagationRShared,
				},
			},
			{
				Source: d.tmpdir,
				Target: "/var/kubernetes",
				Type:   "bind",
				BindOptions: &mount.BindOptions{
					Propagation: mount.PropagationRShared,
				},
			},
		},
		Privileged:  true,
		SecurityOpt: []string{"seccomp:unconfined"},
	}, nil, "")
	if err != nil {
		return err
	}
	d.containerID = resp.ID
	log.Printf("Created container %s", d.containerID)

	if err = d.docker.ContainerStart(ctx, d.containerID, types.ContainerStartOptions{}); err != nil {
		return err
	}
	log.Printf("Started container %s", d.containerID)

	// Synchronously wait for the cluster to come up. This lets us annotate events for debugging, plus we need a couple fields.
	//TODO(Q-Lee): publishing a time-to-running metric would be nice.

	// Wait for the container to be running, and get its ipaddr.
	ipaddr := ""
	pollCh := time.NewTicker(time.Duration(10) * time.Second).C
RunningLoop:
	for {
		select {
		case <-d.control.Interrupt.C:
			return fmt.Errorf("timed out waiting for container %s to start", d.containerID)
		case <-pollCh:
			resp, err := d.docker.ContainerInspect(ctx, d.containerID)
			if err != nil {
				continue
			}
			if resp.NetworkSettings == nil {
				continue
			}
			if resp.NetworkSettings.DefaultNetworkSettings.IPAddress == "" {
				continue
			}
			if resp.ContainerJSONBase == nil {
				continue
			}
			if resp.ContainerJSONBase.State == nil {
				continue
			}
			if !resp.ContainerJSONBase.State.Running {
				continue
			}
			ipaddr = resp.NetworkSettings.DefaultNetworkSettings.IPAddress
			break RunningLoop
		}
	}
	log.Printf("Container %s is running with ipaddr %s", d.containerID, ipaddr)

	// Wait for kubeadm to write the kubecfg file. This is done on the master, and placed on a shared mount.
KubecfgLoop:
	for {
		select {
		case <-d.control.Interrupt.C:
			return fmt.Errorf("timed out waiting for kubeconfig file from cluster container %s", d.containerID)
		case <-pollCh:
			// We don't have permissions to read the kubecfg until after it's written, but we shouldn't rely on that coincidence.
			info, err := os.Stat(d.kubecfg)
			if err != nil {
				continue
			}
			if info.Size() < 1 {
				continue
			}
			// LoadFromFile returns the default config if a file is empty. Don't load until the file has been written.
			cfg, err := clientcmd.LoadFromFile(d.kubecfg)
			if err != nil {
				continue
			}
			// We're hitting the cluster from behind a NAT, so change the server field to hit the outer container.
			for _, cluster := range cfg.Clusters {
				if cluster == nil {
					continue
				}
				cluster.Server = "https://" + ipaddr + ":443"
			}

			// Write out the kubecfg for the e2e tests to consume.
			f, err := ioutil.TempFile("/tmp", "k8s-dind-kubecfg-")
			if err != nil {
				return err
			}
			err = clientcmd.WriteToFile(*cfg, f.Name())
			if err != nil {
				return err
			}
			d.RealKubecfg = f.Name()

			// The easiest thing to do is just load the altereted kubecfg from the file we wrote.
			config, err := clientcmd.BuildConfigFromFlags("", d.RealKubecfg)
			if err != nil {
				return err
			}
			d.apiserver, err = kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}
			break KubecfgLoop
		}
	}
	log.Printf("RealKubecfg file published at %s", d.RealKubecfg)

ApiserverLoop:
	// Wait for the apiserver to become available.
	for {
		select {
		case <-d.control.Interrupt.C:
			return fmt.Errorf("timed out waiting for apiserver from cluster container %s", d.containerID)
		case <-pollCh:
			statuses, err := d.isApiServerUp()
			if err != nil {
				continue
			}
			log.Printf("Statuses: %v", statuses)
			break ApiserverLoop
		}
	}
	log.Printf("apiserver is now available")

	return nil
}

// IsUp returns nil if the apiserver is running, or the error received while checking.
func (d *DindDeployer) IsUp() error {
	_, err := d.isApiServerUp()
	return err
}

func (d *DindDeployer) isApiServerUp() (*v1.ComponentStatusList, error) {
	if d.apiserver == nil {
		return nil, fmt.Errorf("no apiserver client available")
	}
	//TODO(Q-Lee): check that relevant components have started. May consider checking addons.
	return d.apiserver.CoreV1().ComponentStatuses().List(metav1.ListOptions{})
}

// DumpClusterLogs is a no-op.
func (d *DindDeployer) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

// TestSetup is a no-op.
func (d *DindDeployer) TestSetup() error {
	return nil
}

// Down stops and removes the cluster container.
func (d *DindDeployer) Down() error {
	if d.containerID == "" {
		return nil
	}
	ctx := context.Background()
	// Stop the container.
	dur := time.Duration(1) * time.Second
	if err := d.docker.ContainerStop(ctx, d.containerID, &dur); err != nil {
		return err
	}
	return d.docker.ContainerRemove(ctx, d.containerID, types.ContainerRemoveOptions{RemoveVolumes: true})
}

// GetClusterCreated returns the start time of the cluster container. If the container doesn't exist, has no start time, or has a malformed start time, then an error is returned.
func (d *DindDeployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	ctx := context.Background()
	resp, err := d.docker.ContainerInspect(ctx, d.containerID)
	if err != nil {
		return time.Time{}, err
	}
	if resp.ContainerJSONBase == nil {
		return time.Time{}, fmt.Errorf("no ContainerJSONBase for container id %s in response %v", d.containerID, resp)
	}
	if resp.ContainerJSONBase.State == nil {
		return time.Time{}, fmt.Errorf("no State for container id %s in response %v", d.containerID, resp)
	}
	return time.Parse(time.RFC3339, resp.ContainerJSONBase.State.StartedAt)
}
