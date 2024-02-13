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
	"golang.org/x/time/rate"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/crier/reporters/criercommonlib"
)

type ReportClient interface {
	// Report reports a Prowjob. The provided logger is already populated with the
	// prowjob name and the reporter name.
	// If a reporter wants to defer reporting, it can return a reconcile.Result with a RequeueAfter
	Report(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error)
	GetName() string
	// ShouldReport determines if a ProwJob should be reported. The provided logger
	// is already populated with the prowjob name and the reporter name.
	ShouldReport(ctx context.Context, log *logrus.Entry, pj *prowv1.ProwJob) bool
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
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers,
			RateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 500*time.Second),
				// 20 qps, 200 bucket size.
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(20), 200)})}).
		Complete(&reconciler{
			pjclientset:       mgr.GetClient(),
			reporter:          reporter,
			enablementChecker: enablementChecker,
		}); err != nil {
		return fmt.Errorf("failed to construct controller: %w", err)
	}

	return nil
}

// Reconcile retrieves each queued item and takes the necessary handler action based off of if
// the item was created or deleted.
func (r *reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logrus.WithField("reporter", r.reporter.GetName()).WithField("key", req.String()).WithField("prowjob", req.Name)
	log.Debug("processing next key")
	result, err := r.reconcile(ctx, log, req)
	if err != nil {
		if criercommonlib.IsUserError(err) {
			log.WithError(err).Debug("Reconciliation failed")
		} else {
			log.WithError(err).Error("Reconciliation failed")
		}
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *reconciler) reconcile(ctx context.Context, log *logrus.Entry, req reconcile.Request) (*reconcile.Result, error) {
	// Limit reconciliation time to 30 minutes. This should more than enough time
	// for any reasonable reporter. Most reporters should set a stricter timeout
	// themselves. This mainly helps avoid leaking reconciliation threads that
	// will never complete.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
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

	if !r.reporter.ShouldReport(ctx, log, &pj) {
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
	pjs, requeue, err := r.reporter.Report(ctx, log, &pj)
	if err != nil {
		if criercommonlib.IsUserError(err) {
			log.WithError(err).Debug("Failed to report job.")
		} else {
			log.WithError(err).Error("Failed to report job.")
		}
		crierMetrics.reportingResults.WithLabelValues(r.reporter.GetName(), ResultError).Inc()
		return nil, fmt.Errorf("failed to report job: %w", err)
	}
	if requeue != nil {
		return requeue, nil
	}

	crierMetrics.reportingResults.WithLabelValues(r.reporter.GetName(), ResultSuccess).Inc()
	log.WithField("job-count", len(pjs)).Info("Reported job(s), now will update pj(s).")
	var lastErr error
	for _, pjob := range pjs {
		if err := criercommonlib.UpdateReportStateWithRetries(ctx, pjob, log, r.pjclientset, r.reporter.GetName()); err != nil {
			log.WithError(err).Error("Failed to update report state on prowjob")
			// The error above is already logged, so it would be duplicated
			// effort to combine all errors to return, only capture the last
			// error should be sufficient.
			lastErr = err
		}
	}

	if pj.Status.CompletionTime != nil {
		latency := time.Now().Unix() - pj.Status.CompletionTime.Unix()
		crierMetrics.latency.WithLabelValues(r.reporter.GetName()).Observe(float64(latency))
		log.WithField("latency", latency).Debug("Report latency.")
	}

	return nil, lastErr
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
