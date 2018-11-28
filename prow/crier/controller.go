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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	clientset "k8s.io/test-infra/prow/client/clientset/versioned"
	pjinformers "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
)

type reportClient interface {
	Report(pj *v1.ProwJob) error
	GetName() string
	ShouldReport(pj *v1.ProwJob) bool
}

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	pjclientset clientset.Interface
	queue       workqueue.RateLimitingInterface
	informer    pjinformers.ProwJobInformer
	reporter    reportClient
	numWorkers  int
	wg          *sync.WaitGroup
}

func NewController(
	pjclientset clientset.Interface,
	queue workqueue.RateLimitingInterface,
	informer pjinformers.ProwJobInformer,
	reporter reportClient,
	numWorkers int,
	wg *sync.WaitGroup) *Controller {
	return &Controller{
		pjclientset: pjclientset,
		queue:       queue,
		informer:    informer,
		reporter:    reporter,
		numWorkers:  numWorkers,
		wg:          wg,
	}
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(stopCh <-chan struct{}) {
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
	for i := 0; i < c.numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	logrus.Infof("Started %d workers", c.numWorkers)
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
	c.wg.Add(1)
	for c.processNextItem() {
	}
	c.wg.Done()
}

func (c *Controller) retry(key interface{}, err error) bool {
	keyRaw := key.(string)
	if c.queue.NumRequeues(key) < 5 {
		logrus.WithError(err).WithField("prowjob", keyRaw).Error("Failed processing item, retrying")
		c.queue.AddRateLimited(key)
	} else {
		logrus.WithError(err).WithField("prowjob", keyRaw).Error("Failed processing item, no more retries")
		c.queue.Forget(key)
	}

	return true
}

func (c *Controller) updateReportState(pj *v1.ProwJob) error {
	pjData, err := json.Marshal(pj)
	if err != nil {
		return fmt.Errorf("error marshal pj: %v", err)
	}

	// update pj report status
	newpj := pj.DeepCopy()
	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if newpj.Status.PrevReportStates == nil {
		newpj.Status.PrevReportStates = map[string]v1.ProwJobState{}
	}
	newpj.Status.PrevReportStates[c.reporter.GetName()] = newpj.Status.State

	newpjData, err := json.Marshal(newpj)
	if err != nil {
		return fmt.Errorf("error marshal new pj: %v", err)
	}

	patch, err := jsonpatch.CreateMergePatch(pjData, newpjData)
	if err != nil {
		return fmt.Errorf("error CreateMergePatch: %v", err)
	}

	logrus.Infof("Created merge patch: %v", string(patch))

	_, err = c.pjclientset.Prow().ProwJobs(pj.Namespace).Patch(pj.Name, types.MergePatchType, patch)
	return err
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
		logrus.WithError(err).WithField("prowjob", keyRaw).Error("invalid resource key")
		c.queue.Forget(key)
		return true
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

		return c.retry(key, err)
	}

	// not belong to the current reporter
	if !c.reporter.ShouldReport(pj) {
		c.queue.Forget(key)
		return true
	}

	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if pj.Status.PrevReportStates == nil {
		pj.Status.PrevReportStates = map[string]v1.ProwJobState{}
	}

	// already reported current state
	if pj.Status.PrevReportStates[c.reporter.GetName()] == pj.Status.State {
		c.queue.Forget(key)
		return true
	}

	logrus.WithField("prowjob", keyRaw).Infof("Will report state : %s", pj.Status.State)

	if err := c.reporter.Report(pj); err != nil {
		logrus.WithError(err).WithField("prowjob", keyRaw).Error("failed to report job")
		return c.retry(key, err)
	}

	logrus.WithField("prowjob", keyRaw).Info("Updated job, now will update pj")

	if err := c.updateReportState(pj); err != nil {
		logrus.WithError(err).WithField("prowjob", keyRaw).Error("failed to update report state")

		// theoretically patch should not have this issue, but in case:
		// it might be out-dated, try to re-fetch pj and try again

		updatedPJ, err := c.pjclientset.Prow().ProwJobs(pj.Namespace).Get(pj.Name, metav1.GetOptions{})
		if err != nil {
			logrus.WithError(err).WithField("prowjob", keyRaw).Error("failed to get prowjob from apiserver")
			c.queue.Forget(key)
			return true
		}

		if err := c.updateReportState(updatedPJ); err != nil {
			// shrug
			logrus.WithError(err).WithField("prowjob", keyRaw).Error("failed to update report state again, give up")
			c.queue.Forget(key)
			return true
		}
	}

	logrus.WithField("prowjob", keyRaw).Infof("Hunky Dory!, pj : %v, state : %s", pj.Spec.Job, pj.Status.State)

	c.queue.Forget(key)
	return true
}
