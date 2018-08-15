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
	"k8s.io/test-infra/kubetest/e2e"
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

// BuildTester returns an object that knows how to test the cluster it deployed.
func (d *Deployer) BuildTester(o *e2e.BuildTesterOptions) (e2e.Tester, error) {
	// Make a tmpdir for the test report.
	// TODO(Q-Lee): perhaps there should be one tmpdir per cluster container, and all else can be subdirs?
	tmpdir, err := ioutil.TempDir("/tmp", "dind-k8s-report-dir-")
	if err != nil {
		return nil, err
	}

	t := e2e.NewGinkgoTester(o)

	t.Seed = 1436380640

	// dind tester sets parallelism a little differently
	t.GinkgoParallel = 1
	t.NumNodes = o.Parallelism
	t.Provider = "skeleton"

	t.Kubeconfig = d.RealKubecfg
	t.FlakeAttempts = 2
	t.SystemdServices = []string{"docker", "kubelet"}
	t.ReportDir = tmpdir

	return t, nil
}

// Deployer stores information necessary to deploy a cluster inside a docker container.
type Deployer struct {
	image       string
	kubecfg     string
	containerID string
	tmpdir      string
	RealKubecfg string
	docker      *client.Client
	control     *process.Control
	apiserver   *kubernetes.Clientset
}

// Deployer implements e2e.TestBuilder, overriding testing
var _ e2e.TestBuilder = &Deployer{}

// NewDeployer instantiates a new Deployer struct with specified args.
//
// kubecfg: path to a ~/.kube/config type file that authenticates to the cluster
// image: name of the dind image to use, will choose a default if empty
// control: used for creating subprocesses.
func NewDeployer(kubecfg, image string, control *process.Control) (*Deployer, error) {
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
	return &Deployer{
		image:   image,
		kubecfg: kubecfg,
		tmpdir:  tmpdir,
		docker:  docker,
		control: control,
	}, nil
}

func (d *Deployer) ensureImage() error {
	// Once Kubernetes is built, we should grab the version for dind.
	if d.image == "" {
		tag, err := GetDockerVersion()
		if err != nil {
			return err
		}
		d.image = "k8s.gcr.io/dind-cluster-amd64:" + tag
	}
	return nil
}

// Up synchronously starts a cluster, or times out.
func (d *Deployer) Up() error {
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
			statuses, err := d.isAPIServerUp()
			if err != nil {
				continue
			}
			log.Printf("Statuses: %v", statuses)
			break ApiserverLoop
		}
	}
	log.Printf("apiserver is now available")

NodeHealthLoop:
	// Wait for the expected number of nodes to be ready.
	for {
		log.Printf("Waiting for nodes....")
		select {
		case <-d.control.Interrupt.C:
			return fmt.Errorf("timed out waiting for nodes to be ready from cluster container %s", d.containerID)
		case <-pollCh:
			nodes, err := d.apiserver.CoreV1().Nodes().List(metav1.ListOptions{})
			if err != nil {
				log.Printf("No response for nodes....")
				continue
			}
			// TODO(Q-Lee): take this from a flag.
			numNodes := 4

			// Make sure we have at least the correct number of nodes.
			if len(nodes.Items) < numNodes {
				log.Printf("Not enough nodes....")
				continue
			}

			// Make sure all nodes are ready.
			healthyNodes := 0
			for _, node := range nodes.Items {
				for _, condition := range node.Status.Conditions {
					if v1.NodeReady == condition.Type && v1.ConditionTrue == condition.Status {
						healthyNodes++
						break
					}
				}
			}

			if healthyNodes < numNodes {
				log.Printf("Not enough healthy nodes....")
				continue
			}
			log.Printf("All %d nodes are now healthy.", numNodes)
			break NodeHealthLoop
		}
	}
	return nil
}

// IsUp returns nil if the apiserver is running, or the error received while checking.
func (d *Deployer) IsUp() error {
	_, err := d.isAPIServerUp()
	return err
}

func (d *Deployer) isAPIServerUp() (*v1.ComponentStatusList, error) {
	if d.apiserver == nil {
		return nil, fmt.Errorf("no apiserver client available")
	}
	//TODO(Q-Lee): check that relevant components have started. May consider checking addons.
	return d.apiserver.CoreV1().ComponentStatuses().List(metav1.ListOptions{})
}

// DumpClusterLogs is a no-op.
func (d *Deployer) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

// TestSetup is a no-op.
func (d *Deployer) TestSetup() error {
	return nil
}

// Down stops and removes the cluster container.
func (d *Deployer) Down() error {
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
func (d *Deployer) GetClusterCreated(gcpProject string) (time.Time, error) {
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
