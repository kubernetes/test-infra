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

// package crier reports finished prowjob status to git providers
package crier

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	pjinformers "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
)

type reportClient interface {
	Report(pj *v1.ProwJob) error
}

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	clientset kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	informer  pjinformers.ProwJobInformer
	reporter  reportClient
}

func NewController(
	clientset kubernetes.Interface,
	queue workqueue.RateLimitingInterface,
	informer pjinformers.ProwJobInformer,
	reporter reportClient) *Controller {
	return &Controller{
		clientset: clientset,
		queue:     queue,
		informer:  informer,
		reporter:  reporter,
	}
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(numWorkers int, stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	logrus.Info("Initiating controller")
	c.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			logrus.WithField("prowjob", key).Infof("Add prowjob")
			if err != nil {
				logrus.WithError(err).Error("Cannot get key from object meta")
				return
			}
			c.queue.Add(key)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			logrus.WithField("prowjob", key).Infof("Update prowjob")
			if err != nil {
				logrus.WithError(err).Error("Cannot get key from object meta")
				return
			}
			c.queue.Add(key)
		},
	})

	// run the informer to start listing and watching resources
	go c.informer.Informer().Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return
	}
	logrus.Info("Controller.Run: cache sync complete")

	// run the runWorker method every second with a stop channel
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	logrus.Infof("Started %d workers", numWorkers)
	<-stopCh
	logrus.Info("Shutting down workers")
}

// HasSynced allows us to satisfy the Controller interface
// by wiring up the informer's HasSynced method to it
func (c *Controller) HasSynced() bool {
	return c.informer.Informer().HasSynced()
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextItem() bool {

	key, quit := c.queue.Get()

	if quit {
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `namespace/name`)
	keyRaw := key.(string)
	namespace, name, err := cache.SplitMetaNamespaceKey(keyRaw)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", keyRaw))
	}

	// take the string key and get the object out of the indexer
	//
	// item will contain the complex object for the resource and
	// exists is a bool that'll indicate whether or not the
	// resource was created (true) or deleted (false)
	//
	// if there is an error in getting the key from the index
	// then we want to retry this particular queue key a certain
	// number of times (5 here) before we forget the queue key
	// and throw an error
	pj, err := c.informer.Lister().ProwJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.WithField("prowjob", keyRaw).Infof("object no longer exist")
			c.queue.Forget(key)
			return true
		}

		if c.queue.NumRequeues(key) < 5 {
			logrus.WithError(err).WithField("prowjob", keyRaw).Error("Failed processing item, retrying")
			c.queue.AddRateLimited(key)
		} else {
			logrus.WithError(err).WithField("prowjob", keyRaw).Error("Failed processing item, no more retries")
			c.queue.Forget(key)
			utilruntime.HandleError(err)
		}
		return true
	}

	if pj.Spec.Report && pj.Status.PrevReportState != pj.Status.State {
		logrus.Infof("Will report here, pj : %v, state : %s", pj.Spec.Job, pj.Status.State)

		// TODO(krzyzacy): we probably should make report async as well
		// we can also leverage this by increase number of workers.
		if err := c.reporter.Report(pj); err == nil {
			pj.Status.PrevReportState = pj.Status.State
		}
	}

	c.queue.Forget(key)
	return true
}
