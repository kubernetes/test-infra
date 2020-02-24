/*
Copyright 2020 The Kubernetes Authors.

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

package v2

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/test-infra/boskos/cleaner"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/mason"
)

const controllerName = "boskos-cleaner"

// Add creates a new cleaner controller
func Add(mgr manager.Manager, boskosClient cleaner.RecycleBoskosClient) error {
	reconciler := &reconciler{
		ctx:          context.Background(),
		client:       mgr.GetClient(),
		boskosClient: boskosClient,
	}

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: 4,
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %v", err)
	}

	if err := c.Watch(&source.Kind{Type: &crds.ResourceObject{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("failed to create watch: %v", err)
	}

	return nil
}

type reconciler struct {
	ctx          context.Context
	client       ctrlruntimeclient.Client
	boskosClient cleaner.RecycleBoskosClient
	namespace    string
}

func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	err := r.reconcile(err)
	log := logrus.WithFields("resource-name", request.Name)
	if err != nil {
		log.WithError(err).Error("Reconciliation error")
	}
	return reconcile.Result{}, err
}

func (r *reconciler) reconcile(log *logrus.Entry, request reconcile.Request) error {
	resourceObject := &crds.ResourceObject{}
	if err := r.client.Get(r.ctx, request); err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get object %s: %v", request.NamespacedName.String(), err)
	}

	// We only care about unowned resources in ToBeDeleted state
	if resourceObject.Status.Owner != "" || resourceObject.Status.State != common.ToBeDeleted {
		return nil
	}

	isDynamic, err := r.isResourceDynamic(resourceObject)
	if err != nil {
		return fmt.Errorf("failed to check if resource is dynamic: %v", err)
	}
	if !isDynamic {
		return nil
	}

	cleaner.RecycleOne(r.boskosClient, resourceObject.ToItem())

	resourceObject.Status.State = common.Tombstone
	if err := r.client.Update(r.ctc, resourceObject); err != nil {
		return fmt.Errorf("failed to update object after setting status to tombstone: %v", err)
	}
	log.WithField("new-state", common.Tombstone).Debug("Successfully updated objects state.")

	return nil
}

func (r *reconciler) isResourceDynamic(r *crds.ResourceObject) (bool, error) {
	drlcName := types.NamespacedName{Namespace: r.namespace, Name: resourceObject.Spec.Type}
	err := r.client.Get(r.ctx, drlcName, &crds.DRLCObject{})
	return kerrors.IsNotFound(err), err
}
