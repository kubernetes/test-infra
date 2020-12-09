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
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type ReportClient interface {
	// Report reports a Prowjob. The provided logger is already populated with the
	// prowjob name and the reporter name.
	// If a reporter wants to defer reporting, it can return a reconcile.Result with a RequeueAfter
	Report(log *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error)
	GetName() string
	// ShouldReport determines if a ProwJob should be reported. The provided logger
	// is already populated with the prowjob name and the reporter name.
	ShouldReport(log *logrus.Entry, pj *prowv1.ProwJob) bool
}

// reconciler struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type reconciler struct {
	pjclientset       ctrlruntimeclient.Client
	reporter          ReportClient
	enablementChecker func(org, repo string) bool
}

// New constructs a new instance of the crier reconciler.
func New(
	mgr manager.Manager,
	reporter ReportClient,
	numWorkers int,
	enablementChecker func(org, repo string) bool,
) error {
	if err := builder.
		ControllerManagedBy(mgr).
		// Is used for metrics, hence must be unique per controller instance
		Named(fmt.Sprintf("crier_%s", reporter.GetName())).
		For(&prowv1.ProwJob{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers}).
		Complete(&reconciler{
			pjclientset:       mgr.GetClient(),
			reporter:          reporter,
			enablementChecker: enablementChecker,
		}); err != nil {
		return fmt.Errorf("failed to construct controller: %w", err)
	}

	return nil
}

func (r *reconciler) updateReportState(ctx context.Context, pj *prowv1.ProwJob, log *logrus.Entry, reportedState prowv1.ProwJobState) error {
	// update pj report status
	newpj := pj.DeepCopy()
	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if newpj.Status.PrevReportStates == nil {
		newpj.Status.PrevReportStates = map[string]prowv1.ProwJobState{}
	}
	newpj.Status.PrevReportStates[r.reporter.GetName()] = reportedState

	if err := r.pjclientset.Patch(ctx, newpj, ctrlruntimeclient.MergeFrom(pj)); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	// Block until the update is in the lister to make sure that events from another controller
	// that also does reporting dont trigger another report because our lister doesn't yet contain
	// the updated Status
	name := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
	if err := wait.Poll(100*time.Millisecond, 3*time.Second, func() (bool, error) {
		if err := r.pjclientset.Get(ctx, name, pj); err != nil {
			return false, err
		}
		if pj.Status.PrevReportStates != nil &&
			pj.Status.PrevReportStates[r.reporter.GetName()] == reportedState {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for updated report status to be in lister: %w", err)
	}
	return nil
}

func (r *reconciler) updateReportStateWithRetries(ctx context.Context, pj *prowv1.ProwJob, log *logrus.Entry) error {
	reportState := pj.Status.State
	log = log.WithFields(logrus.Fields{
		"prowjob":   pj.Name,
		"jobName":   pj.Spec.Job,
		"jobStatus": reportState,
	})
	// We have to retry here, if we return we lose the information that we already reported this job.
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get it first, this is very cheap
		name := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
		if err := r.pjclientset.Get(ctx, name, pj); err != nil {
			return err
		}
		// Must not wrap until we have kube 1.19, otherwise the RetryOnConflict won't recognize conflicts
		// correctly
		return r.updateReportState(ctx, pj, log, reportState)
	}); err != nil {
		// Very subpar, we will report again. But even if we didn't do that now, we would do so
		// latest when crier gets restarted. In an ideal world, all reporters are idempotent and
		// reporting has no cost.
		return fmt.Errorf("failed to update report state on prowjob: %w", err)
	}

	log.Info("Successfully updated report state on prowjob")
	return nil
}

// Reconcile retrieves each queued item and takes the necessary handler action based off of if
// the item was created or deleted.
func (r *reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logrus.WithField("reporter", r.reporter.GetName()).WithField("key", req.String()).WithField("prowjob", req.Name)
	log.Debug("processing next key")
	result, err := r.reconcile(ctx, log, req)
	if err != nil {
		log.WithError(err).Error("Reconciliation failed")
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *reconciler) reconcile(ctx context.Context, log *logrus.Entry, req reconcile.Request) (*reconcile.Result, error) {

	var pj prowv1.ProwJob
	if err := r.pjclientset.Get(ctx, req.NamespacedName, &pj); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("object no longer exist")
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get prowjob %s: %w", req.String(), err)
	}

	if !r.shouldHandle(&pj) {
		return nil, nil
	}

	log = log.WithField("jobName", pj.Spec.Job)

	if !pj.Spec.Report || !r.reporter.ShouldReport(log, &pj) {
		return nil, nil
	}

	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if pj.Status.PrevReportStates == nil {
		pj.Status.PrevReportStates = map[string]prowv1.ProwJobState{}
	}

	// already reported current state
	if pj.Status.PrevReportStates[r.reporter.GetName()] == pj.Status.State {
		log.Trace("Already reported")
		return nil, nil
	}

	log = log.WithField("jobStatus", pj.Status.State)
	log.Info("Will report state")
	pjs, requeue, err := r.reporter.Report(log, &pj)
	if err != nil {
		log.WithError(err).Error("failed to report job")
		return nil, fmt.Errorf("failed to report job: %w", err)
	}
	if requeue != nil {
		return requeue, nil
	}

	log.Info("Reported job(s), now will update pj(s)")
	for _, pjob := range pjs {
		if err := r.updateReportStateWithRetries(ctx, pjob, log); err != nil {
			log.WithError(err).Error("Failed to update report state on prowjob")
			return nil, err
		}
	}

	return nil, nil
}

func (r *reconciler) shouldHandle(pj *prowv1.ProwJob) bool {
	refs := pj.Spec.ExtraRefs
	if pj.Spec.Refs != nil {
		refs = append(refs, *pj.Spec.Refs)
	}
	if len(refs) == 0 {
		return true
	}

	// It is possible to have conflicting settings here, we choose
	// to report if in doubt because reporting multiple times is
	// better than not reporting at all.
	var enabled bool
	for _, ref := range refs {
		if r.enablementChecker(ref.Org, ref.Repo) {
			enabled = true
			break
		}
	}

	return enabled
}
