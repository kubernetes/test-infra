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

package artifact_uploader

import (
	"bytes"
	"fmt"
	"path"
	"time"

	"github.com/golang/glog"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

const (
	// ContainerLogDir is the prefix under which we place
	// container logs in cloud storage
	ContainerLogDir = "logs"
)

// item describes a container that we saw finish execution
type item struct {
	namespace     string
	podName       string
	containerName string
	prowJobId     string
}

func NewController(client core.CoreV1Interface, prowJobClient *kube.Client, gcsConfig *gcsupload.Options) Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	optionsModifier := func(options *metav1.ListOptions) {
		req, _ := labels.NewRequirement(kube.ProwJobIDLabel, selection.Exists, []string{})
		options.LabelSelector = req.String()
	}
	podListWatcher := cache.NewFilteredListWatchFromClient(client.RESTClient(), "pods", api.NamespaceAll, optionsModifier)
	indexer, informer := cache.NewIndexerInformer(podListWatcher, &api.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old interface{}, new interface{}) {
			oldPod := old.(*api.Pod)
			newPod := new.(*api.Pod)

			containers := findFinishedContainers(oldPod.Status.InitContainerStatuses, newPod.Status.InitContainerStatuses)
			containers = append(containers, findFinishedContainers(oldPod.Status.ContainerStatuses, newPod.Status.ContainerStatuses)...)

			for _, container := range containers {
				queue.Add(item{
					namespace:     newPod.Namespace,
					podName:       newPod.Name,
					containerName: container,
					prowJobId:     newPod.Labels[kube.ProwJobIDLabel],
				})
			}

		},
	}, cache.Indexers{})

	return Controller{
		queue:         queue,
		indexer:       indexer,
		informer:      informer,
		client:        client,
		prowJobClient: prowJobClient,
		gcsConfig:     gcsConfig,
	}
}

func findFinishedContainers(old, new []api.ContainerStatus) []string {
	var containerNames []string
	for _, oldInitContainer := range old {
		if oldInitContainer.Name == kube.TestContainerName {
			// logs from the test container will be uploaded by the
			// sidecar, so we do not need to worry about them here
			continue
		}
		for _, newInitContainer := range new {
			// we need to take action if we see a container that is
			// terminated that was not terminated last time we saw it
			if oldInitContainer.Name == newInitContainer.Name &&
				oldInitContainer.State.Terminated == nil && newInitContainer.State.Terminated != nil {
				containerNames = append(containerNames, newInitContainer.Name)
			}
		}
	}
	return containerNames
}

type Controller struct {
	queue    workqueue.RateLimitingInterface
	indexer  cache.Indexer
	informer cache.Controller

	client        core.CoreV1Interface
	prowJobClient *kube.Client

	gcsConfig *gcsupload.Options
}

func (c *Controller) Run(numWorkers int, stopCh chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()
	go c.informer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

// runWorker runs the worker until the queue signals to quit
func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

// processNextItem attempts to upload container logs to GCS
func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	workItem := key.(item)

	prowJob, err := c.prowJobClient.GetProwJob(workItem.prowJobId)
	if err != nil {
		c.handleErr(err, workItem)
		return true
	}
	spec := downwardapi.NewJobSpec(prowJob.Spec, prowJob.Status.BuildID, prowJob.Name)

	result := c.client.Pods(workItem.namespace).GetLogs(workItem.podName, &api.PodLogOptions{Container: workItem.containerName}).Do()
	if err := result.Error(); err != nil {
		c.handleErr(err, workItem)
		return true
	}

	// error is checked above
	log, _ := result.Raw()
	var target string
	if workItem.podName == workItem.prowJobId {
		target = path.Join(ContainerLogDir, fmt.Sprintf("%s.txt", workItem.containerName))
	} else {
		target = path.Join(ContainerLogDir, workItem.podName, fmt.Sprintf("%s.txt", workItem.containerName))
	}
	data := gcs.DataUpload(bytes.NewReader(log))
	if err := c.gcsConfig.Run(&spec, map[string]gcs.UploadFunc{target: data}); err != nil {
		c.handleErr(err, workItem)
		return true
	}
	c.queue.Forget(key)
	return true
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key item) {
	if c.queue.NumRequeues(key) < 5 {
		glog.Infof("Error uploading logs for container %v in pod %v: %v", key.containerName, key.podName, err)
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	glog.Infof("Giving up on upload of logs for container %v in pod %v: %v", key.containerName, key.podName, err)
}
