/*
Copyright 2024 The Kubernetes Authors.

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

package scheduler

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/scheduler/strategy"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const ControllerName = "scheduler"

func Add(mgr controllerruntime.Manager, cfg config.Getter, numWorkers int) error {
	predicates := predicate.NewPredicateFuncs(func(object client.Object) bool {
		pj, isPJ := object.(*prowv1.ProwJob)
		return isPJ && pj.Status.State == prowv1.SchedulingState && pj.Spec.Agent == prowv1.KubernetesAgent
	})

	strategy := strategy.Get(cfg())
	reconciler := NewReconciler(mgr.GetClient(), strategy)
	if err := controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&prowv1.ProwJob{}).
		WithEventFilter(predicates).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers}).
		Complete(reconciler); err != nil {
		return fmt.Errorf("failed to construct controller: %w", err)
	}

	return nil
}

type Reconciler struct {
	pjClient client.Client
	strategy strategy.Interface
	log      *logrus.Entry
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithField("request", request)

	pj := &prowv1.ProwJob{}
	if err := r.pjClient.Get(ctx, request.NamespacedName, pj); err != nil {
		if !kerrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("get prowjob %s: %w", request.Name, err)
		}
		return reconcile.Result{}, nil
	}

	log = log.WithField("job", pj.Spec.Job)

	result, err := r.strategy.Schedule(ctx, pj)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("schedule prowjob %s: %w", request.Name, err)
	}
	log.WithField("cluster", result.Cluster).Info("Cluster assigned")

	// Don't mess the cache up
	scheduled := pj.DeepCopy()
	scheduled.Spec.Cluster = result.Cluster
	scheduled.Status.State = prowv1.TriggeredState

	if err := r.pjClient.Patch(ctx, scheduled, client.MergeFrom(pj.DeepCopy())); err != nil {
		return reconcile.Result{}, fmt.Errorf("patch prowjob: %w", err)
	}

	return reconcile.Result{}, nil
}

func NewReconciler(pjClient client.Client, strategy strategy.Interface) *Reconciler {
	return &Reconciler{
		pjClient: pjClient,
		strategy: strategy,
		log:      logrus.NewEntry(logrus.StandardLogger()).WithField("controller", ControllerName),
	}
}
