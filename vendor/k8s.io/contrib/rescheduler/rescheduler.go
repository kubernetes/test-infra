/*
Copyright 2016 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"os"
	"time"

	ca_simulator "k8s.io/contrib/cluster-autoscaler/simulator"
	kube_utils "k8s.io/contrib/cluster-autoscaler/utils/kubernetes"
	kube_api "k8s.io/kubernetes/pkg/api"
	kube_record "k8s.io/kubernetes/pkg/client/record"
	kube_restclient "k8s.io/kubernetes/pkg/client/restclient"
	kube_client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	kubectl_util "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"

	"github.com/golang/glog"
	flag "github.com/spf13/pflag"
)

const (
	criticalPodAnnotation      = "scheduler.alpha.kubernetes.io/critical-pod"
	criticalAddonsOnlyTaintKey = "CriticalAddonsOnly"
)

var (
	flags = flag.NewFlagSet(
		`rescheduler: rescheduler --running-in-cluster=true`,
		flag.ExitOnError)

	inCluster = flags.Bool("running-in-cluster", true,
		`Optional, if this controller is running in a kubernetes cluster, use the
		 pod secrets for creating a Kubernetes client.`)

	contentType = flags.String("kube-api-content-type", "application/vnd.kubernetes.protobuf",
		`Content type of requests sent to apiserver.`)

	housekeepingInterval = flags.Duration("housekeeping-interval", 10*time.Second,
		`How often rescheduler takes actions.`)

	systemNamespace = flags.String("system-namespace", kube_api.NamespaceSystem,
		`Namespace to watch for critical addons.`)

	initialDelay = flags.Duration("initial-delay", 2*time.Minute,
		`How long should rescheduler wait after start to make sure
		 all critical addons had a chance to start.`)

	podScheduledTimeout = flags.Duration("pod-scheduled-timeout", 10*time.Minute,
		`How long should rescheduler wait for critical pod to be scheduled
		 after evicting pods to make a spot for it.`)
)

func main() {
	glog.Infof("Running Rescheduler")

	flags.Parse(os.Args)

	// TODO(piosz): figure our a better way of verifying cluster stabilization here.
	time.Sleep(*initialDelay)

	kubeClient, err := createKubeClient(flags, *inCluster)
	if err != nil {
		glog.Fatalf("Failed to create kube client: %v", err)
	}

	recorder := createEventRecorder(kubeClient)

	predicateChecker, err := ca_simulator.NewPredicateChecker(kubeClient)
	if err != nil {
		glog.Fatalf("Failed to create predicate checker: %v", err)
	}

	unschedulablePodLister := kube_utils.NewUnschedulablePodLister(kubeClient, *systemNamespace)
	nodeLister := kube_utils.NewNodeLister(kubeClient)

	// TODO(piosz): consider reseting this set once every few hours.
	podsBeingProcessed := NewPodSet()

	releaseAllTaints(kubeClient, nodeLister, podsBeingProcessed)

	for {
		select {
		case <-time.After(*housekeepingInterval):
			{
				allUnschedulablePods, err := unschedulablePodLister.List()
				if err != nil {
					glog.Errorf("Failed to list unscheduled pods: %v", err)
					continue
				}

				criticalPods := filterCriticalPods(allUnschedulablePods, podsBeingProcessed)

				if len(criticalPods) > 0 {
					for _, pod := range criticalPods {
						glog.Infof("Critical pod %s is unschedulable. Trying to find a spot for it.", podId(pod))
						nodes, err := nodeLister.List()
						if err != nil {
							glog.Errorf("Failed to list nodes: %v", err)
							continue
						}

						node := findNodeForPod(kubeClient, predicateChecker, nodes, pod)
						if node == nil {
							glog.Errorf("Pod %s can't be scheduled on any existing node.", podId(pod))
							recorder.Eventf(pod, kube_api.EventTypeNormal, "PodDoestFitAnyNode",
								"Critical pod %s doesn't fit on any node.", podId(pod))
							continue
						}
						glog.Infof("Trying to place the pod on node %v", node.Name)

						err = prepareNodeForPod(kubeClient, recorder, predicateChecker, node, pod)
						if err != nil {
							glog.Warningf("%+v", err)
						} else {
							podsBeingProcessed.Add(pod)
							go waitForScheduled(kubeClient, podsBeingProcessed, pod)
						}
					}
				}

				releaseAllTaints(kubeClient, nodeLister, podsBeingProcessed)
			}
		}
	}
}

func waitForScheduled(client *kube_client.Client, podsBeingProcessed *podSet, pod *kube_api.Pod) {
	glog.Infof("Waiting for pod %s to be scheduled", podId(pod))
	err := wait.Poll(time.Second, *podScheduledTimeout, func() (bool, error) {
		p, err := client.Pods(pod.Namespace).Get(pod.Name)
		if err != nil {
			glog.Warningf("Error while getting pod %s: %v", podId(pod), err)
			return false, nil
		}
		return p.Spec.NodeName != "", nil
	})
	if err != nil {
		glog.Warningf("Timeout while waiting for pod %s to be scheduled after %v.", podId(pod), *podScheduledTimeout)
	} else {
		glog.Infof("Pod %v was successfully scheduled.", podId(pod))
	}
	podsBeingProcessed.Remove(pod)
}

func createKubeClient(flags *flag.FlagSet, inCluster bool) (*kube_client.Client, error) {
	var config *kube_restclient.Config
	var err error
	if inCluster {
		config, err = kube_restclient.InClusterConfig()
	} else {
		clientConfig := kubectl_util.DefaultClientConfig(flags)
		config, err = clientConfig.ClientConfig()
	}
	if err != nil {
		fmt.Errorf("error connecting to the client: %v", err)
	}
	config.ContentType = *contentType
	return kube_client.NewOrDie(config), nil
}

func createEventRecorder(client *kube_client.Client) kube_record.EventRecorder {
	eventBroadcaster := kube_record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(client.Events(""))
	return eventBroadcaster.NewRecorder(kube_api.EventSource{Component: "rescheduler"})
}

func releaseAllTaints(client *kube_client.Client, nodeLister *kube_utils.ReadyNodeLister, podsBeingProcessed *podSet) {
	nodes, err := nodeLister.List()
	if err != nil {
		glog.Warningf("Cannot release taints - error while listing nodes: %v", err)
		return
	}

	for _, node := range nodes {
		taints, err := kube_api.GetTaintsFromNodeAnnotations(node.Annotations)
		if err != nil {
			glog.Warningf("Error while getting Taints for node %v: %v", node.Name, err)
			continue
		}

		newTaints := make([]kube_api.Taint, 0)
		for _, taint := range taints {
			if taint.Key == criticalAddonsOnlyTaintKey && !podsBeingProcessed.HasId(taint.Value) {
				glog.Infof("Releasing taint %+v on node %v", taint, node.Name)
			} else {
				newTaints = append(newTaints, taint)
			}
		}

		if len(newTaints) != len(taints) {
			taintsJson, err := json.Marshal(newTaints)
			if err != nil {
				glog.Warningf("Error while releasing taints on node %v: %v", node.Name, err)
				continue
			}

			node.Annotations[kube_api.TaintsAnnotationKey] = string(taintsJson)
			_, err = client.Nodes().Update(node)
			if err != nil {
				glog.Warningf("Error while releasing taints on node %v: %v", node.Name, err)
			} else {
				glog.Infof("Successfully released all taints on node %v", node.Name)
			}
		}
	}
}

// The caller of this function must remove the taint if this function returns error.
func prepareNodeForPod(client *kube_client.Client, recorder kube_record.EventRecorder, predicateChecker *ca_simulator.PredicateChecker, originalNode *kube_api.Node, criticalPod *kube_api.Pod) error {
	// Operate on a copy of the node to ensure pods running on the node will pass CheckPredicates below.
	node, err := copyNode(originalNode)
	if err != nil {
		return fmt.Errorf("Error while copying node: %v", err)
	}
	err = addTaint(client, originalNode, podId(criticalPod))
	if err != nil {
		return fmt.Errorf("Error while adding taint: %v", err)
	}

	requiredPods, otherPods, err := groupPods(client, node)
	if err != nil {
		return err
	}

	nodeInfo := schedulercache.NewNodeInfo(requiredPods...)
	nodeInfo.SetNode(node)

	// check whether critical pod still fit
	if err := predicateChecker.CheckPredicates(criticalPod, nodeInfo); err != nil {
		return fmt.Errorf("Pod %s doesn't fit to node %v: %v", podId(criticalPod), node.Name, err)
	}
	requiredPods = append(requiredPods, criticalPod)
	nodeInfo = schedulercache.NewNodeInfo(requiredPods...)
	nodeInfo.SetNode(node)

	for _, p := range otherPods {
		if err := predicateChecker.CheckPredicates(p, nodeInfo); err != nil {
			glog.Infof("Pod %s will be deleted in order to schedule critical pod %s.", podId(p), podId(criticalPod))
			recorder.Eventf(p, kube_api.EventTypeNormal, "DeletedByRescheduler",
				"Deleted by rescheduler in order to schedule critical pod %s.", podId(criticalPod))
			// TODO(piosz): add better support of graceful deletion
			delErr := client.Pods(p.Namespace).Delete(p.Name, kube_api.NewDeleteOptions(10))
			if delErr != nil {
				return fmt.Errorf("Failed to delete pod %s: %v", podId(p), delErr)
			}
		} else {
			newPods := append(nodeInfo.Pods(), p)
			nodeInfo = schedulercache.NewNodeInfo(newPods...)
			nodeInfo.SetNode(node)
		}
	}

	// TODO(piosz): how to reset scheduler backoff?
	return nil
}

func copyNode(node *kube_api.Node) (*kube_api.Node, error) {
	objCopy, err := kube_api.Scheme.DeepCopy(node)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*kube_api.Node)
	if !ok {
		return nil, fmt.Errorf("expected Node, got %#v", objCopy)
	}
	return copied, nil
}

func addTaint(client *kube_client.Client, node *kube_api.Node, value string) error {
	taints, err := kube_api.GetTaintsFromNodeAnnotations(node.Annotations)
	if err != nil {
		return err
	}

	taint := kube_api.Taint{
		Key:    criticalAddonsOnlyTaintKey,
		Value:  value,
		Effect: kube_api.TaintEffectNoSchedule,
	}
	taints = append(taints, taint)

	taintsJson, err := json.Marshal(taints)
	if err != nil {
		return err
	}

	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[kube_api.TaintsAnnotationKey] = string(taintsJson)
	_, err = client.Nodes().Update(node)
	if err != nil {
		return err
	}
	return nil
}

// Currently the logic choose a random node which satisfies requirements (a critical pod fits there).
// TODO(piosz): add a prioritization to this logic
func findNodeForPod(client *kube_client.Client, predicateChecker *ca_simulator.PredicateChecker, nodes []*kube_api.Node, pod *kube_api.Pod) *kube_api.Node {
	for _, node := range nodes {
		// ignore nodes with taints
		if err := checkTaints(node); err != nil {
			glog.Warningf("Skipping node %v due to %v", node.Name, err)
		}

		requiredPods, _, err := groupPods(client, node)
		if err != nil {
			glog.Warningf("Skipping node %v due to error: %v", node.Name, err)
			continue
		}

		nodeInfo := schedulercache.NewNodeInfo(requiredPods...)
		nodeInfo.SetNode(node)

		if err := predicateChecker.CheckPredicates(pod, nodeInfo); err == nil {
			return node
		}
	}
	return nil
}

func checkTaints(node *kube_api.Node) error {
	taints, err := kube_api.GetTaintsFromNodeAnnotations(node.Annotations)
	if err != nil {
		return fmt.Errorf("error while getting taints: %v", err)
	}
	for _, taint := range taints {
		if taint.Key == criticalAddonsOnlyTaintKey {
			return fmt.Errorf("CriticalAddonsOnly taint with value: %v", taint.Value)
		}
	}
	return nil
}

// groupPods divides pods running on <node> into those which can't be deleted and the others
func groupPods(client *kube_client.Client, node *kube_api.Node) ([]*kube_api.Pod, []*kube_api.Pod, error) {
	podsOnNode, err := client.Pods(kube_api.NamespaceAll).List(
		kube_api.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name})})
	if err != nil {
		return []*kube_api.Pod{}, []*kube_api.Pod{}, err
	}

	requiredPods := make([]*kube_api.Pod, 0)
	otherPods := make([]*kube_api.Pod, 0)
	for i := range podsOnNode.Items {
		pod := &podsOnNode.Items[i]

		creatorRef, err := ca_simulator.CreatorRefKind(pod)
		if err != nil {
			return []*kube_api.Pod{}, []*kube_api.Pod{}, err
		}

		if ca_simulator.IsMirrorPod(pod) || creatorRef == "DaemonSet" || isCriticalPod(pod) {
			requiredPods = append(requiredPods, pod)
		} else {
			otherPods = append(otherPods, pod)
		}
	}

	return requiredPods, otherPods, nil
}

func filterCriticalPods(allPods []*kube_api.Pod, podsBeingProcessed *podSet) []*kube_api.Pod {
	criticalPods := []*kube_api.Pod{}
	for _, pod := range allPods {
		if isCriticalPod(pod) && !podsBeingProcessed.Has(pod) {
			criticalPods = append(criticalPods, pod)
		}
	}
	return criticalPods
}

func isCriticalPod(pod *kube_api.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[criticalPodAnnotation]
	return found
}
