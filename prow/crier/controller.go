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

// Package crier reports finished prowjob status to git providers.
package crier

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	clientset "k8s.io/test-infra/prow/client/clientset/versioned"
	pjinformers "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
)

type reportClient interface {
	Report(pj *v1.ProwJob) ([]*v1.ProwJob, error)
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

// NewController constructs a new instance of the crier controller.
func NewController(
	pjclientset clientset.Interface,
	queue workqueue.RateLimitingInterface,
	informer pjinformers.ProwJobInformer,
	reporter reportClient,
	numWorkers int) *Controller {
	return &Controller{
		pjclientset: pjclientset,
		queue:       queue,
		informer:    informer,
		reporter:    reporter,
		numWorkers:  numWorkers,
		wg:          &sync.WaitGroup{},
	}
}

// Run is the main path of execution for the controller loop.
func (c *Controller) Run(ctx context.Context) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()

	logrus.Info("Initiating controller")
	c.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			logrus.WithField("prowjob", key).Debug("Add prowjob")
			if err != nil {
				logrus.WithError(err).Error("Cannot get key from object meta")
				return
			}
			c.queue.AddRateLimited(key)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			logrus.WithField("prowjob", key).Debug("Update prowjob")
			if err != nil {
				logrus.WithError(err).Error("Cannot get key from object meta")
				return
			}
			c.queue.AddRateLimited(key)
		},
	})

	// run the informer to start listing and watching resources
	go c.informer.Informer().Run(ctx.Done())

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(ctx.Done(), c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return
	}
	logrus.Info("Controller.Run: cache sync complete")

	// run the runWorker method every second with a stop channel
	for i := 0; i < c.numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, ctx.Done())
	}

	logrus.Infof("Started %d workers", c.numWorkers)
	<-ctx.Done()
	logrus.Info("Shutting down workers")
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	c.queue.ShutDown()
	c.wg.Wait()
}

// HasSynced allows us to satisfy the Controller interface by wiring up the informer's HasSynced
// method to it.
func (c *Controller) HasSynced() bool {
	return c.informer.Informer().HasSynced()
}

// runWorker executes the loop to process new items added to the queue.
func (c *Controller) runWorker() {
	c.wg.Add(1)
	for c.processNextItem() {
	}
	c.wg.Done()
}

func (c *Controller) retry(key interface{}, err error) bool {
	keyRaw := key.(string)
	if c.queue.NumRequeues(key) < 5 {
		logrus.WithError(err).WithField("prowjob", keyRaw).Info("Failed processing item, retrying")
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

	if len(patch) == 0 {
		logrus.Warnf("Empty merge patch: pjData: %s, newpjData: %s", string(pjData), string(newpjData))
	}

	logrus.Infof("Created merge patch: %v", string(patch))

	_, err = c.pjclientset.ProwV1().ProwJobs(pj.Namespace).Patch(pj.Name, types.MergePatchType, patch)
	if err != nil {
		return err
	}

	// Block until the update is in the lister to make sure that events from another controller
	// that also does reporting dont trigger another report because our lister doesn't yet contain
	// the updated Status
	if err := wait.Poll(time.Second, 3*time.Second, func() (bool, error) {
		pj, err := c.informer.Lister().ProwJobs(newpj.Namespace).Get(newpj.Name)
		if err != nil {
			return false, err
		}
		if pj.Status.PrevReportStates != nil &&
			newpj.Status.PrevReportStates[c.reporter.GetName()] == newpj.Status.State {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for updated report status to be in lister: %v", err)
	}
	return nil
}

// processNextItem retrieves each queued item and takes the necessary handler action based off of if
// the item was created or deleted.
func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		logrus.Debug("Queue already shut down, exiting processNextItem")
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `namespace/name`)
	keyRaw := key.(string)
	logrus.WithField("key", keyRaw).Debug("processing next key")

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
	readOnlyPJ, err := c.informer.Lister().ProwJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.WithField("prowjob", keyRaw).Info("object no longer exist")
			c.queue.Forget(key)
			return true
		}

		return c.retry(key, err)
	}
	pj := readOnlyPJ.DeepCopy()

	// not belong to the current reporter
	if !pj.Spec.Report || !c.reporter.ShouldReport(pj) {
		c.queue.Forget(key)
		return true
	}

	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if pj.Status.PrevReportStates == nil {
		pj.Status.PrevReportStates = map[string]v1.ProwJobState{}
	}

	// already reported current state
	if pj.Status.PrevReportStates[c.reporter.GetName()] == pj.Status.State {
		logrus.WithField("prowjob", keyRaw).Info("Already reported")
		c.queue.Forget(key)
		return true
	}

	logrus.WithField("prowjob", keyRaw).Infof("Will report state : %s", pj.Status.State)
	pjs, err := c.reporter.Report(pj)
	if err != nil {
		fields := logrus.Fields{
			"prowjob":   keyRaw,
			"jobName":   pj.Name,
			"jobStatus": pj.Status,
		}
		logrus.WithError(err).WithFields(fields).Error("failed to report job")
		return c.retry(key, err)
	}

	logrus.WithField("prowjob", keyRaw).Info("Updated job, now will update pj")
	for _, pjob := range pjs {
		if err := c.updateReportState(pjob); err != nil {
			logrus.WithError(err).WithField("prowjob", keyRaw).Error("failed to update report state")

			// theoretically patch should not have this issue, but in case:
			// it might be out-dated, try to re-fetch pj and try again

			updatedPJ, err := c.pjclientset.ProwV1().ProwJobs(pjob.Namespace).Get(pjob.Name, metav1.GetOptions{})
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

		logrus.WithField("prowjob", keyRaw).Infof("Hunky Dory!, pj : %v, state : %s", pjob.Spec.Job, pjob.Status.State)
	}
	c.queue.Forget(key)
	return true
}
