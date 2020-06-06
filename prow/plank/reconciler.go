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

package plank

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

const ControllerName = "plank"

func Add(
	mgr controllerruntime.Manager,
	buildMgrs map[string]controllerruntime.Manager,
	cfg config.Getter,
	totURL string,
	additionalSelector string,
) error {
	return add(mgr, buildMgrs, cfg, totURL, additionalSelector, nil, nil, 10)
}

func add(
	mgr controllerruntime.Manager,
	buildMgrs map[string]controllerruntime.Manager,
	cfg config.Getter,
	totURL string,
	additionalSelector string,
	overwriteReconcile reconcileFunc,
	predicateCallack func(bool),
	numWorkers int,
) error {
	predicate, err := predicates(additionalSelector, predicateCallack)
	if err != nil {
		return fmt.Errorf("failed to construct predicate: %w", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(&prowv1.ProwJob{}, prowJobIndexName, prowJobIndexer(cfg().ProwJobNamespace)); err != nil {
		return fmt.Errorf("failed to add indexer: %w", err)
	}

	blder := controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&prowv1.ProwJob{}).
		WithEventFilter(predicate).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers})

	r := newReconciler(mgr.GetClient(), overwriteReconcile, cfg, totURL)
	for buildCluster, buildClusterMgr := range buildMgrs {
		blder = blder.Watches(
			source.NewKindWithCache(&corev1.Pod{}, buildClusterMgr.GetCache()),
			podEventRequestMapper(cfg().ProwJobNamespace))
		r.buildClients[buildCluster] = buildClusterMgr.GetClient()
	}

	if err := blder.Complete(r); err != nil {
		return fmt.Errorf("failed to build controller: %w", err)
	}

	if err := mgr.Add(manager.RunnableFunc(r.syncMetrics)); err != nil {
		return fmt.Errorf("failed to add metrics runnable to manager: %w", err)
	}

	return nil
}

func newReconciler(pjClient ctrlruntimeclient.Client, overwriteReconcile reconcileFunc, cfg config.Getter, totURL string) *reconciler {
	return &reconciler{
		ctx:                context.Background(),
		pjClient:           pjClient,
		buildClients:       map[string]ctrlruntimeclient.Client{},
		overwriteReconcile: overwriteReconcile,
		log:                logrus.NewEntry(logrus.StandardLogger()).WithField("controller", ControllerName),
		config:             cfg,
		totURL:             totURL,
		clock:              clock.RealClock{},
		serializationLocks: &shardedLock{
			mapLock: &sync.Mutex{},
			locks:   map[string]*semaphore.Weighted{},
		},
	}
}

type reconcileFunc = func(reconcile.Request) (reconcile.Result, error)

type reconciler struct {
	ctx                context.Context
	pjClient           ctrlruntimeclient.Client
	buildClients       map[string]ctrlruntimeclient.Client
	overwriteReconcile reconcileFunc
	log                *logrus.Entry
	config             config.Getter
	totURL             string
	clock              clock.Clock
	serializationLocks *shardedLock
}

type shardedLock struct {
	mapLock *sync.Mutex
	locks   map[string]*semaphore.Weighted
}

func (s *shardedLock) getLock(key string) *semaphore.Weighted {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	if _, exists := s.locks[key]; !exists {
		s.locks[key] = semaphore.NewWeighted(1)
	}
	return s.locks[key]
}

func (r *reconciler) syncMetrics(stop <-chan struct{}) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			pjs := &prowv1.ProwJobList{}
			if err := r.pjClient.List(r.ctx, pjs, optAllProwJobs()); err != nil {
				r.log.WithError(err).Error("failed to list prowjobs for metrics")
				continue
			}
			kube.GatherProwJobMetrics(pjs.Items)
		}
	}
}

func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	if r.overwriteReconcile != nil {
		return r.overwriteReconcile(request)
	}
	return r.defaultReconcile(request)
}

func (r *reconciler) defaultReconcile(request reconcile.Request) (reconcile.Result, error) {
	pj := &prowv1.ProwJob{}
	if err := r.pjClient.Get(r.ctx, request.NamespacedName, pj); err != nil {
		if !kerrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get prowjob %s: %v", request.Name, err)
		}

		// Objects can be deleted from the API while being in our workqueue
		return reconcile.Result{}, nil
	}

	// TODO: Terminal errors for unfixable cases like missing build clusters
	// and not return an error to prevent requeuing?
	res, err := r.serializeIfNeeded(pj)
	if res == nil {
		res = &reconcile.Result{}
	}
	if err != nil {
		r.log.WithError(err).Error("Reconciliation failed")
	}
	return *res, err
}

// serializeIfNeeded serializes the reconciliation of Jobs that have a MaxConcurrency setting, otherwise
// multiple reconciliations of the same job may race and not properly respect that setting.
func (r *reconciler) serializeIfNeeded(pj *prowv1.ProwJob) (*reconcile.Result, error) {
	if pj.Spec.MaxConcurrency == 0 {
		return r.reconcile(pj)
	}

	sema := r.serializationLocks.getLock(pj.Spec.Job)
	// Use TryAcquire to avoid blocking workers waiting for the lock
	if !sema.TryAcquire(1) {
		return &reconcile.Result{RequeueAfter: time.Second}, nil
	}
	defer sema.Release(1)
	return r.reconcile(pj)
}

func (r *reconciler) reconcile(pj *prowv1.ProwJob) (*reconcile.Result, error) {
	// terminateDupes first, as that might reduce cluster load and prevent us
	// from doing pointless work.
	if err := r.terminateDupes(pj); err != nil {
		return nil, fmt.Errorf("terminateDupes failed: %w", err)
	}

	switch pj.Status.State {
	case prowv1.PendingState:
		return nil, r.syncPendingJob(pj)
	case prowv1.TriggeredState:
		return r.syncTriggeredJob(pj)
	case prowv1.AbortedState:
		return nil, r.syncAbortedJob(pj)
	}

	return nil, nil
}

func (r *reconciler) terminateDupes(pj *prowv1.ProwJob) error {
	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(r.ctx, pjs, optNotCompletedProwJobs()); err != nil {
		return fmt.Errorf("failed to list prowjobs: %v", err)
	}

	return pjutil.TerminateOlderJobs(r.pjClient, r.log, pjs.Items, r.terminateDupesCleanup)
}

func (r *reconciler) terminateDupesCleanup(pj prowv1.ProwJob) error {
	client, ok := r.buildClients[pj.ClusterAlias()]
	if !ok {
		return fmt.Errorf("no client for cluster %q present", pj.ClusterAlias())
	}
	podToDelete := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.config().PodNamespace,
			Name:      pj.Name,
		},
	}
	if err := client.Delete(r.ctx, podToDelete); err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", podToDelete.Namespace, podToDelete.Name, pj.ClusterAlias(), err)
	}
	return nil
}

// syncPendingJob syncs jobs for which we already created the test workload
func (r *reconciler) syncPendingJob(pj *prowv1.ProwJob) error {
	prevPJ := pj.DeepCopy()

	pod, podExists, err := r.pod(pj)
	if err != nil {
		return err
	}

	if !podExists {
		// Pod is missing. This can happen in case the previous pod was deleted manually or by
		// a rescheduler. Start a new pod.
		id, pn, err := r.startPod(pj)
		if err != nil {
			if !isRequestError(err) {
				return fmt.Errorf("error starting pod %s: %v", pod.Name, err)
			}
			pj.Status.State = prowv1.ErrorState
			pj.SetComplete()
			pj.Status.Description = "Job cannot be processed."
			r.log.WithFields(pjutil.ProwJobFields(pj)).WithError(err).Warning("Unprocessable pod.")
		} else {
			pj.Status.BuildID = id
			pj.Status.PodName = pn
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pod is missing, starting a new pod")
		}
	} else {

		switch pod.Status.Phase {
		case corev1.PodUnknown:
			// Pod is in Unknown state. This can happen if there is a problem with
			// the node. Delete the old pod, this will fire an event that triggers
			// a new reconciliation in which we will re-create the pod.
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pod is in unknown state, deleting & restarting pod")
			client, ok := r.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("unknown pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}

			r.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
			return client.Delete(r.ctx, pod)

		case corev1.PodSucceeded:
			// Pod succeeded. Update ProwJob and talk to GitHub.
			pj.SetComplete()
			pj.Status.State = prowv1.SuccessState
			pj.Status.Description = "Job succeeded."

		case corev1.PodFailed:
			if pod.Status.Reason == Evicted {
				// Pod was evicted.
				if pj.Spec.ErrorOnEviction {
					// ErrorOnEviction is enabled, complete the PJ and mark it as errored.
					pj.SetComplete()
					pj.Status.State = prowv1.ErrorState
					pj.Status.Description = "Job pod was evicted by the cluster."
					break
				}
				// ErrorOnEviction is disabled. Delete the pod now and recreate it in
				// the next resync.
				client, ok := r.buildClients[pj.ClusterAlias()]
				if !ok {
					return fmt.Errorf("evicted pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
				}
				r.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
				return client.Delete(r.ctx, pod)
			}
			// Pod failed. Update ProwJob, talk to GitHub.
			pj.SetComplete()
			pj.Status.State = prowv1.FailureState
			pj.Status.Description = "Job failed."

		case corev1.PodPending:
			maxPodPending := r.config().Plank.PodPendingTimeout.Duration
			maxPodUnscheduled := r.config().Plank.PodUnscheduledTimeout.Duration
			if pod.Status.StartTime.IsZero() {
				if time.Since(pod.CreationTimestamp.Time) >= maxPodUnscheduled {
					// Pod is stuck in unscheduled state longer than maxPodUncheduled
					// abort the job, and talk to GitHub
					pj.SetComplete()
					pj.Status.State = prowv1.ErrorState
					pj.Status.Description = "Pod scheduling timeout."
					r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Marked job for stale unscheduled pod as errored.")
					break
				}
			} else if time.Since(pod.Status.StartTime.Time) >= maxPodPending {
				// Pod is stuck in pending state longer than maxPodPending
				// abort the job, and talk to GitHub
				pj.SetComplete()
				pj.Status.State = prowv1.ErrorState
				pj.Status.Description = "Pod pending timeout."
				r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Marked job for stale pending pod as errored.")
				break
			}
			// Pod is running. Do nothing.
			return nil
		case corev1.PodRunning:
			maxPodRunning := r.config().Plank.PodRunningTimeout.Duration
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodRunning {
				// Pod is still running. Do nothing.
				return nil
			}

			// Pod is stuck in running state longer than maxPodRunning
			// abort the job, and talk to GitHub
			pj.SetComplete()
			pj.Status.State = prowv1.AbortedState
			pj.Status.Description = "Pod running timeout."
			client, ok := r.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("running pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}
			if err := client.Delete(r.ctx, pod); err != nil {
				return fmt.Errorf("failed to delete pod %s that was in running timeout: %v", pod.Name, err)
			}
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Deleted stale running pod.")
		default:
			// other states, ignore
			return nil
		}
	}

	pj.Status.URL, err = pjutil.JobURL(r.config().Plank, *pj, r.log)
	if err != nil {
		r.log.WithFields(pjutil.ProwJobFields(pj)).WithError(err).Warn("failed to get jobURL")
	}

	if prevPJ.Status.State != pj.Status.State {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}

	if err := r.pjClient.Patch(r.ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
		return fmt.Errorf("patching prowjob: %w", err)
	}

	return nil
}

// syncTriggeredJob syncs jobs that do not yet have an associated test workload running
func (r *reconciler) syncTriggeredJob(pj *prowv1.ProwJob) (*reconcile.Result, error) {
	prevPJ := pj.DeepCopy()

	var id, pn string

	pod, podExists, err := r.pod(pj)
	if err != nil {
		return nil, err
	}
	// We may end up in a state where the pod exists but the prowjob is not
	// updated to pending if we successfully create a new pod in a previous
	// sync but the prowjob update fails. Simply ignore creating a new pod
	// and rerun the prowjob update.
	if podExists {
		id = getPodBuildID(pod)
		pn = pod.ObjectMeta.Name
	} else {
		// Do not start more jobs than specified and check again later.
		canExecuteConcurrently, err := r.canExecuteConcurrently(pj)
		if err != nil {
			return nil, fmt.Errorf("canExecuteConcurrently: %v", err)
		}
		if !canExecuteConcurrently {
			return &reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// We haven't started the pod yet. Do so.
		id, pn, err = r.startPod(pj)
		if err != nil {
			if !isRequestError(err) {
				return nil, fmt.Errorf("error starting pod: %v", err)
			}
			pj.Status.State = prowv1.ErrorState
			pj.SetComplete()
			pj.Status.Description = "Job cannot be processed."
			logrus.WithField("job", pj.Spec.Job).WithError(err).Warning("Unprocessable pod.")
		}
	}

	if pj.Status.State == prowv1.TriggeredState {
		// BuildID needs to be set before we execute the job url template.
		pj.Status.BuildID = id
		now := metav1.NewTime(r.clock.Now())
		pj.Status.PendingTime = &now
		pj.Status.State = prowv1.PendingState
		pj.Status.PodName = pn
		pj.Status.Description = "Job triggered."
		pj.Status.URL, err = pjutil.JobURL(r.config().Plank, *pj, r.log)
		if err != nil {
			r.log.WithFields(pjutil.ProwJobFields(pj)).WithError(err).Warn("failed to get jobURL")
		}
	}

	if prevPJ.Status.State != pj.Status.State {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	if err := r.pjClient.Patch(r.ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
		return nil, fmt.Errorf("patch prowjob: %w", err)
	}

	// If the job has a MaxConcurrency setting, we must block here until we observe the state transition in our cache,
	// otherwise subequent reconciliations for a different run of the same job might incorrectly conclude that they
	// can run because that decision is made based on the data in the cache.
	if pj.Spec.MaxConcurrency == 0 {
		return nil, nil
	}
	nn := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
	state := pj.Status.State
	if err := wait.Poll(100*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := r.pjClient.Get(r.ctx, nn, pj); err != nil {
			return false, fmt.Errorf("failed to get prowjob: %w", err)
		}
		return pj.Status.State == state, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for cached prowjob %s to get into state %s: %w", nn.String(), state, err)
	}

	return nil, nil
}

// syncAbortedJob syncs jobs that got aborted because their result isn't needed anymore,
// for example because of a new push or because a pull request got closed.
func (r *reconciler) syncAbortedJob(pj *prowv1.ProwJob) error {

	buildClient, ok := r.buildClients[pj.ClusterAlias()]
	if !ok {
		return fmt.Errorf("no build client available for cluster %s", pj.ClusterAlias())
	}

	// Just optimistically delete and swallow the potential 404
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      pj.Name,
		Namespace: r.config().PodNamespace,
	}}
	if err := buildClient.Delete(r.ctx, pod); err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", pod.Namespace, pod.Name, pj.ClusterAlias(), err)
	}

	originalPJ := pj.DeepCopy()
	pj.SetComplete()
	return r.pjClient.Patch(r.ctx, pj, ctrlruntimeclient.MergeFrom(originalPJ))
}

func (r *reconciler) pod(pj *prowv1.ProwJob) (*corev1.Pod, bool, error) {
	buildClient, buildClientExists := r.buildClients[pj.ClusterAlias()]
	if !buildClientExists {
		// TODO: Use terminal error type to prevent requeuing, this wont be fixed without
		// a restart
		return nil, false, fmt.Errorf("no build client found for cluster %q", pj.ClusterAlias())
	}

	pod := &corev1.Pod{}
	name := types.NamespacedName{
		Namespace: r.config().PodNamespace,
		Name:      pj.Name,
	}

	if err := buildClient.Get(r.ctx, name, pod); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get pod: %v", err)
	}

	return pod, true, nil
}

func (r *reconciler) startPod(pj *prowv1.ProwJob) (string, string, error) {
	buildID, err := r.getBuildID(pj.Spec.Job)
	if err != nil {
		return "", "", fmt.Errorf("error getting build ID: %v", err)
	}

	pod, err := decorate.ProwJobToPod(*pj, buildID)
	if err != nil {
		return "", "", err
	}
	pod.Namespace = r.config().PodNamespace

	client, ok := r.buildClients[pj.ClusterAlias()]
	if !ok {
		// TODO: Terminal error to prevent requeuing
		return "", "", fmt.Errorf("unknown cluster alias %q", pj.ClusterAlias())
	}
	err = client.Create(r.ctx, pod)
	r.log.WithFields(pjutil.ProwJobFields(pj)).Debug("Create Pod.")
	if err != nil {
		return "", "", err
	}

	// We must block until we see the pod, otherwise a new reconciliation may be triggered that tries to create
	// the pod because its not in the cache yet, errors with IsAlreadyExists and sets the prowjob to failed
	podName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	if err := wait.Poll(100*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := client.Get(r.ctx, podName, pod); err != nil {
			if kerrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("failed to get pod %s in cluster %s: %w", podName.String(), pj.ClusterAlias(), err)
		}
		return true, nil
	}); err != nil {
		return "", "", fmt.Errorf("failed waiting for new pod %s in cluster %s  appear in cache: %w", podName.String(), pj.ClusterAlias(), err)
	}

	return buildID, pod.Name, nil
}

func (r *reconciler) getBuildID(name string) (string, error) {
	return pjutil.GetBuildID(name, r.totURL)
}

// canExecuteConcurrently determines if the cocurrency settings allow our job
// to be started. We start jobs with a limited concurrency in order, oldest
// first. This allows us to get away without any global locking by just looking
// at the jobs in the cluster.
func (r *reconciler) canExecuteConcurrently(pj *prowv1.ProwJob) (bool, error) {

	if max := r.config().Plank.MaxConcurrency; max > 0 {
		pjs := &prowv1.ProwJobList{}
		if err := r.pjClient.List(r.ctx, pjs, optNotCompletedProwJobs()); err != nil {
			return false, fmt.Errorf("failed to list prowjobs: %w", err)
		}
		// The list contains our own ProwJob
		running := len(pjs.Items) - 1
		if running >= max {
			r.log.WithFields(pjutil.ProwJobFields(pj)).Infof("Not starting another job, already %d running.", running)
			return false, nil
		}
	}

	if pj.Spec.MaxConcurrency == 0 {
		return true, nil
	}

	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(r.ctx, pjs, optNotCompletedProwJobsNamed(pj.Spec.Job)); err != nil {
		return false, fmt.Errorf("failed listing prowjobs: %w:", err)
	}
	r.log.Infof("got %d not completed with same name", len(pjs.Items))

	var olderMatchingPJs int
	for _, foundPJ := range pjs.Items {
		// Ignore self here.
		if foundPJ.UID == pj.UID {
			continue
		}

		if foundPJ.CreationTimestamp.Before(&pj.CreationTimestamp) {
			olderMatchingPJs++
		}

	}

	if olderMatchingPJs >= pj.Spec.MaxConcurrency {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			Debugf("Not starting another instance of %s, already %d older instances waiting and %d is the limit",
				pj.Spec.Job, olderMatchingPJs, pj.Spec.MaxConcurrency)
		return false, nil
	}

	return true, nil
}

func predicatesFromFilter(filter func(m metav1.Object, r runtime.Object) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return filter(e.Meta, e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return filter(e.Meta, e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filter(e.MetaNew, e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return filter(e.Meta, e.Object)
		},
	}

}

func predicates(additionalSelector string, callback func(bool)) (predicate.Predicate, error) {
	rawSelector := fmt.Sprintf("%s=true", kube.CreatedByProw)
	if additionalSelector != "" {
		rawSelector = fmt.Sprintf("%s,%s", rawSelector, additionalSelector)
	}
	selector, err := labels.Parse(rawSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selector %s: %w", rawSelector, err)
	}

	return predicatesFromFilter(func(m metav1.Object, r runtime.Object) bool {
		result := func() bool {
			pj, ok := r.(*prowv1.ProwJob)
			if !ok {
				// We ignore pods that do not match our selector
				return selector.Matches(labels.Set(m.GetLabels()))
			}

			// We can ignore completed prowjobs
			if pj.Complete() {
				return false
			}

			return pj.Spec.Agent == prowv1.KubernetesAgent
		}()
		if callback != nil {
			callback(result)
		}
		return result
	}), nil
}

func podEventRequestMapper(prowJobNamespace string) handler.EventHandler {
	return &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(mo handler.MapObject) []controllerruntime.Request {
			return []controllerruntime.Request{{NamespacedName: ctrlruntimeclient.ObjectKey{
				Namespace: prowJobNamespace,
				Name:      mo.Meta.GetName(),
			}},
			}
		}),
	}
}

const (
	// prowJobIndexName is the name of an index that
	// holds all ProwJobs that are in the correct namespace
	// and use the Kubernetes agent
	prowJobIndexName = "plank-prow-jobs"
	// prowJobIndexKeyAll is the indexKey for all ProwJobs
	prowJobIndexKeyAll = "all"
	// prowJobIndexKeyCompleted is the indexKey for not
	// completed ProwJobs
	prowJobIndexKeyNotCompleted = "not-completed"
)

func prowJobIndexKeyNotCompletedByName(jobName string) string {
	return fmt.Sprintf("not-completed-%s", jobName)
}

func prowJobIndexer(prowJobNamespace string) ctrlruntimeclient.IndexerFunc {
	return func(o runtime.Object) []string {
		pj := o.(*prowv1.ProwJob)
		if pj.Namespace != prowJobNamespace || pj.Spec.Agent != prowv1.KubernetesAgent {
			return nil
		}

		if !pj.Complete() {
			return []string{
				prowJobIndexKeyAll,
				prowJobIndexKeyNotCompleted,
				prowJobIndexKeyNotCompletedByName(pj.Spec.Job),
			}
		}

		return []string{prowJobIndexKeyAll}
	}
}

func optAllProwJobs() ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingField(prowJobIndexName, prowJobIndexKeyAll)
}

func optNotCompletedProwJobs() ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingField(prowJobIndexName, prowJobIndexKeyNotCompleted)
}

func optNotCompletedProwJobsNamed(name string) ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingField(prowJobIndexName, prowJobIndexKeyNotCompletedByName(name))
}
