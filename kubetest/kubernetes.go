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
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"
)

// kubectlGetNodes lists nodes by executing kubectl get nodes, parsing the output into a nodeList object
func kubectlGetNodes(cmd string) (*nodeList, error) {
	if cmd == "" {
		cmd = "kubectl"
	}
	o, err := control.Output(exec.Command(cmd, "get", "nodes", "-ojson"))
	if err != nil {
		log.Printf("kubectl get nodes failed: %s\n%s", wrapError(err).Error(), string(o))
		return nil, err
	}

	nodes := &nodeList{}
	if err := json.Unmarshal(o, nodes); err != nil {
		return nil, fmt.Errorf("error parsing kubectl get nodes output: %v", err)
	}

	return nodes, nil
}

// isReady checks if the node has a Ready Condition that is True
func isReady(node *node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

// waitForReadyNodes polls the nodes until we see at least desiredCount that are Ready
// We can also pass requiredConsecutiveSuccesses to require that we see this N times in a row
func waitForReadyNodes(desiredCount int, timeout time.Duration, requiredConsecutiveSuccesses int) error {
	stop := time.Now().Add(timeout)

	consecutiveSuccesses := 0
	for {
		if time.Now().After(stop) {
			break
		}

		nodes, err := kubectlGetNodes("")
		if err != nil {
			log.Printf("kubectl get nodes failed, sleeping: %v", err)
			consecutiveSuccesses = 0
			time.Sleep(30 * time.Second)
			continue
		}
		readyNodes := countReadyNodes(nodes)
		if readyNodes >= desiredCount {
			consecutiveSuccesses++
			if consecutiveSuccesses >= requiredConsecutiveSuccesses {
				log.Printf("%d ready nodes found, %d sequential successes - cluster is ready",
					readyNodes,
					consecutiveSuccesses)
				return nil
			}
			log.Printf("%d ready nodes found, waiting for %d sequential successes (success count = %d)",
				readyNodes,
				requiredConsecutiveSuccesses,
				consecutiveSuccesses)
			time.Sleep(2 * time.Second)
		} else {
			consecutiveSuccesses = 0
			log.Printf("%d (ready nodes) < %d (requested instances), sleeping", readyNodes, desiredCount)
			time.Sleep(30 * time.Second)
		}
	}
	return fmt.Errorf("waiting for ready nodes timed out")
}

// countReadyNodes returns the number of nodes that have isReady == true
func countReadyNodes(nodes *nodeList) int {
	var ready []*node
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if isReady(node) {
			ready = append(ready, node)
		}
	}
	return len(ready)
}

func kubectlGetPods(ctx context.Context, namespace string, labelSelector []string) (*podList, error) {
	args := []string{"-n", namespace, "get", "pods", "-ojson"}
	for _, ls := range labelSelector {
		args = append(args, "-l", ls)
	}
	o, err := control.Output(exec.CommandContext(ctx, "kubectl", args...))
	if err != nil {
		log.Printf("kubectl get pods failed: %s\n%s", wrapError(err).Error(), string(o))
		return nil, err
	}

	pods := &podList{}
	if err := json.Unmarshal(o, pods); err != nil {
		return nil, fmt.Errorf("error parsing kubectl get pods output: %v", err)
	}

	return pods, nil
}

// nodeList is a simplified version of the v1.NodeList API type
type nodeList struct {
	Items []node `json:"items"`
}

// node is a simplified version of the v1.Node API type
type node struct {
	Metadata metadata   `json:"metadata"`
	Status   nodeStatus `json:"status"`
}

// nodeStatus is a simplified version of the v1.NodeStatus API type
type nodeStatus struct {
	Addresses  []nodeAddress   `json:"addresses"`
	Conditions []nodeCondition `json:"conditions"`
}

// nodeAddress is a simplified version of the v1.NodeAddress API type
type nodeAddress struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

// nodeCondition is a simplified version of the v1.NodeCondition API type
type nodeCondition struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Type    string `json:"type"`
}

// metadata is a simplified version of the kubernetes metadata types
type metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata metadata `json:"metadata"`
	Spec     podSpec  `json:"spec"`
}

type podSpec struct {
	NodeName string `json:"nodeName"`
}
