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

package cluster

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Cluster holds configuration info about the nested cluster.
type Cluster struct {
	numNodes         int
	healthyNodes     int
	apiserverHealthy bool

	externalProxy   string
	image           string
	kubecfg         string
	masterConfigDir string
	networkID       string
	networkName     string
	nodeImageFile   string
	version         string
	nodeIDs         []string

	apiserver *kubernetes.Clientset
	docker    *client.Client
	timeout   time.Duration
}

// New instantiates a new cluster.
//
// Uses NewEnvClient() to create the docker client.
// Assumes kubecfg lives at admin.conf in masterConfigDir.
//
// Returns an error if it cannot create the docker client.
func New(numNodes int, externalProxy, image, masterConfigDir, networkName, nodeImageFile, version string, timeout time.Duration) (*Cluster, error) {
	docker, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	return &Cluster{
		numNodes:        numNodes,
		externalProxy:   externalProxy,
		docker:          docker,
		image:           image,
		kubecfg:         masterConfigDir + "/admin.conf",
		masterConfigDir: masterConfigDir,
		networkName:     networkName,
		nodeImageFile:   nodeImageFile,
		timeout:         timeout,
		version:         version,
	}, nil
}

// Up synchronously starts a cluster, reporting progress.
func (c *Cluster) Up(loadImage bool) error {
	end := time.Now().Add(c.timeout)
	if loadImage {
		glog.V(2).Info("Loading node image")
		if err := c.loadImage(); err != nil {
			return err
		}
	}
	glog.V(2).Info("Creating docker objects")
	if err := c.createDockerObjects(); err != nil {
		return err
	}
	glog.V(2).Info("Starting containers")
	if err := c.startContainers(); err != nil {
		return err
	}
	glog.V(2).Info("Waiting for apiserver")
	if err := c.waitForCluster(end); err != nil {
		return err
	}
	return nil
}

func (c *Cluster) loadImage() error {
	ctx := context.Background()
	file, err := os.Open(c.nodeImageFile)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	resp, err := c.docker.ImageLoad(ctx, reader, true)
	defer resp.Body.Close()
	return err
}

// Create makes all docker objects in the dind cluster.
func (c *Cluster) createDockerObjects() error {
	// First, create the docker network.
	if err := c.createNetwork(); err != nil {
		return err
	}

	// Create the master node.
	if err := c.createMaster("172.18.0.2"); err != nil {
		return err
	}

	// Create the worker nodes
	for i := 0; i < c.numNodes-1; i++ {
		// There are only 255 IPs in the block, so we can't exceed that. This needs to be pulled out for ipv6.
		suffix := i + 3
		if suffix > 255 {
			return fmt.Errorf("Too many nodes, cannot allocate IPs")
		}
		ip := fmt.Sprintf("172.18.0.%d", suffix)
		if err := c.createWorker(ip); err != nil {
			return err
		}
	}
	return nil
}

// Start begins all containers in the dind cluster.
func (c *Cluster) startContainers() error {
	ctx := context.Background()
	for _, id := range c.nodeIDs {
		if err := c.docker.ContainerStart(ctx, id, types.ContainerStartOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) createNetwork() error {
	ctx := context.Background()

	resp, err := c.docker.NetworkCreate(ctx, c.networkName, types.NetworkCreate{
		Driver:     "bridge",
		Scope:      "local",
		EnableIPv6: false,
		IPAM: &network.IPAM{
			Config: []network.IPAMConfig{
				{
					Subnet:  "172.18.0.0/16",
					Gateway: "172.18.0.1",
				},
			},
		},
	})

	if err != nil {
		return err
	}
	c.networkID = resp.ID
	glog.V(2).Infof("Created network with warning %v", resp.Warning)
	return nil
}

// createNode creates a docker container, and returns the docker ID and any errors.
func (c *Cluster) createNode(entrypoint []string, mounts []mount.Mount, ntwk network.EndpointSettings, ports nat.PortMap, exposedPorts nat.PortSet) error {
	ctx := context.Background()

	resp, err := c.docker.ContainerCreate(ctx, &container.Config{
		Image:        c.image + ":" + c.version,
		Entrypoint:   entrypoint,
		ExposedPorts: exposedPorts,
	}, &container.HostConfig{
		CapAdd:       []string{"SYS_ADMIN"},
		Mounts:       mounts,
		NetworkMode:  container.NetworkMode(c.networkName),
		PortBindings: ports,
		Privileged:   true,
		SecurityOpt:  []string{"seccomp:unconfined"},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			c.networkName: &ntwk,
		},
	}, "")
	if err == nil {
		c.nodeIDs = append(c.nodeIDs, resp.ID)
		glog.V(2).Infof("Created node with warnings %s", resp.Warnings)
	}
	if err != nil {
		glog.Errorf("Failed to create master with mounts: %v", mounts)
	}
	return err
}

func (c *Cluster) createWorker(ip string) error {
	entrypoint := []string{"/init-wrapper.sh", "worker"}
	mounts := []mount.Mount{
		{
			Source: "/lib/modules",
			Target: "/lib/modules",
			Type:   "bind",
			BindOptions: &mount.BindOptions{
				Propagation: mount.PropagationRShared,
			},
		},
	}
	ntwk := network.EndpointSettings{
		IPAddress: ip,
		NetworkID: c.networkID,
	}
	return c.createNode(entrypoint, mounts, ntwk, nil, nil)
}

func (c *Cluster) createMaster(ip string) error {
	entrypoint := []string{"/init-wrapper.sh", "master", c.externalProxy}
	mounts := []mount.Mount{
		{
			Source: "/lib/modules",
			Target: "/lib/modules",
			Type:   "bind",
			BindOptions: &mount.BindOptions{
				Propagation: mount.PropagationRShared,
			},
		},
		{
			Source: c.masterConfigDir,
			Target: "/etc/kubernetes",
			Type:   "bind",
			BindOptions: &mount.BindOptions{
				Propagation: mount.PropagationRShared,
			},
		},
	}
	ntwk := network.EndpointSettings{
		IPAddress: ip,
		NetworkID: c.networkID,
	}

	ports := nat.PortMap{
		"6443/tcp": []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: "443",
			},
		},
	}
	exposedPorts := nat.PortSet{
		"6443/tcp": {},
	}
	return c.createNode(entrypoint, mounts, ntwk, ports, exposedPorts)
}

// waitForCluster waits for a cluster to start with a timeout, reporting milestones along the way.
func (c *Cluster) waitForCluster(timeoutTime time.Time) error {
	var lastErr error
	for {
		time.Sleep(time.Duration(1) * time.Second)
		if time.Now().After(timeoutTime) {
			return fmt.Errorf("Timed out waiting for the cluster to come up, last error: %v", lastErr)
		}

		// First, try parsing a kubecfg file. This may not have been created yet.
		if c.apiserver == nil {
			config, err := clientcmd.BuildConfigFromFlags("", c.kubecfg)
			if err != nil {
				glog.V(4).Infof("Couldn't build config: %v", err)
				lastErr = err
				continue
			}
			apiserver, err := kubernetes.NewForConfig(config)
			if err != nil {
				glog.V(4).Infof("Couldn't build k8s client: %v", err)
				lastErr = err
				continue
			}
			c.apiserver = apiserver
			glog.Infof("Acquired kubecfg file %q.", c.kubecfg)
		}

		// Wait for the apiserver to come up, and to report healthy component statuses.
		if !c.apiserverHealthy {
			statuses, err := c.apiserver.CoreV1().ComponentStatuses().List(metav1.ListOptions{})
			if err != nil {
				glog.V(4).Infof("Couldn't get response from apiserver: %v", err)
				lastErr = err
				continue
			}
			// TODO: check individual statuses.
			c.apiserverHealthy = true
			glog.Infof("The apiserver is up, and components are healthy: %v", statuses)
		}

		// Wait for the expected number of nodes to be ready.
		if c.healthyNodes != c.numNodes {
			// Make sure we have at least the correct number of nodes.
			nodes, err := c.apiserver.CoreV1().Nodes().List(metav1.ListOptions{})
			if err != nil {
				glog.V(4).Infof("Couldn't get nodes from apiserver: %v", err)
				lastErr = err
				continue
			}
			if len(nodes.Items) < c.numNodes {
				glog.V(4).Infof("Only %d of %d nodes are reporting.", len(nodes.Items), c.numNodes)
				lastErr = fmt.Errorf("Expected %d nodes, got %d", c.numNodes, len(nodes.Items))
				continue
			}

			// Make sure all nodes are ready.
			c.healthyNodes = 0
			for _, node := range nodes.Items {
				for _, condition := range node.Status.Conditions {
					if v1.NodeReady == condition.Type && v1.ConditionTrue == condition.Status {
						c.healthyNodes++
						break
					}
				}
			}
			glog.Infof("%d of %d nodes are ready.", c.healthyNodes, c.numNodes)
			if c.healthyNodes < c.numNodes {
				lastErr = fmt.Errorf("Expected %d healthy nodes, got %d", c.numNodes, c.healthyNodes)
				continue
			}
			break
		}
	}
	return nil
}
