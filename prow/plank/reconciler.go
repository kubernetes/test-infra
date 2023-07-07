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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/clock"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	kubernetesreporterapi "k8s.io/test-infra/prow/crier/reporters/gcs/kubernetes/api"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/version"
)

const ControllerName = "plank"

// PodStatus constants
const (
	Evicted = "Evicted"
)

// NodeStatus constants
const (
	// NodeUnreachablePodReason is the reason on a pod when its state cannot be confirmed as kubelet is unresponsive
	// on the node it is (was) running.
	NodeUnreachablePodReason = "NodeLost"
)

func Add(
	mgr controllerruntime.Manager,
	buildMgrs map[string]controllerruntime.Manager,
	knownClusters sets.String,
	cfg config.Getter,
	opener io.Opener,
	totURL string,
	additionalSelector string,
) error {
	return add(mgr, buildMgrs, knownClusters, cfg, opener, totURL, additionalSelector, nil, nil, 10)
}

func add(
	mgr controllerruntime.Manager,
	buildMgrs map[string]controllerruntime.Manager,
	knownClusters sets.String,
	cfg config.Getter,
	opener io.Opener,
	totURL string,
	additionalSelector string,
	overwriteReconcile reconcile.Func,
	predicateCallack func(bool),
	numWorkers int,
) error {
	predicate, err := predicates(additionalSelector, predicateCallack)
	if err != nil {
		return fmt.Errorf("failed to construct predicate: %w", err)
	}

	ctx := context.Background()
	if err := mgr.GetFieldIndexer().IndexField(ctx, &prowv1.ProwJob{}, prowJobIndexName, prowJobIndexer(cfg().ProwJobNamespace)); err != nil {
		return fmt.Errorf("failed to add indexer: %w", err)
	}

	blder := controllerruntime.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&prowv1.ProwJob{}).
		WithEventFilter(predicate).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers})

	r := newReconciler(ctx, mgr.GetClient(), overwriteReconcile, cfg, opener, totURL)
	for buildCluster, buildClusterMgr := range buildMgrs {
		r.log.WithFields(logrus.Fields{
			"buildCluster": buildCluster,
			"host":         buildClusterMgr.GetConfig().Host,
		}).Debug("creating client")
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

	if err := mgr.Add(manager.RunnableFunc(r.syncClusterStatus(time.Minute, knownClusters))); err != nil {
		return fmt.Errorf("failed to add cluster status runnable to manager: %w", err)
	}

	return nil
}

func newReconciler(ctx context.Context, pjClient ctrlruntimeclient.Client, overwriteReconcile reconcile.Func, cfg config.Getter, opener io.Opener, totURL string) *reconciler {
	return &reconciler{
		pjClient:           pjClient,
		buildClients:       map[string]ctrlruntimeclient.Client{},
		overwriteReconcile: overwriteReconcile,
		log:                logrus.NewEntry(logrus.StandardLogger()).WithField("controller", ControllerName),
		config:             cfg,
		opener:             opener,
		totURL:             totURL,
		clock:              clock.RealClock{},
		serializationLocks: &shardedLock{
			mapLock: &sync.Mutex{},
			locks:   map[string]*semaphore.Weighted{},
		},
	}
}

type reconciler struct {
	pjClient           ctrlruntimeclient.Client
	buildClients       map[string]ctrlruntimeclient.Client
	overwriteReconcile reconcile.Func
	log                *logrus.Entry
	config             config.Getter
	opener             io.Opener
	totURL             string
	clock              clock.WithTickerAndDelayedExecution
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

func (r *reconciler) syncMetrics(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			pjs := &prowv1.ProwJobList{}
			if err := r.pjClient.List(ctx, pjs, optAllProwJobs()); err != nil {
				r.log.WithError(err).Error("failed to list prowjobs for metrics")
				continue
			}
			kube.GatherProwJobMetrics(r.log, pjs.Items)
		}
	}
}

type ClusterStatus string

const (
	ClusterStatusUnreachable ClusterStatus = "Unreachable"
	ClusterStatusReachable   ClusterStatus = "Reachable"
	ClusterStatusNoManager   ClusterStatus = "No-Manager"
)

func (r *reconciler) syncClusterStatus(interval time.Duration, knownClusters sets.String) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				location := r.config().Plank.BuildClusterStatusFile
				if location == "" {
					continue
				}
				parsedPath, err := prowv1.ParsePath(location)
				if err != nil {
					r.log.WithError(err).Errorf("Failed to parse cluster status file location: %q.", location)
					continue
				}
				// prowv1.ParsePath prepends `Path` with `/`, trim it
				bucket, subPath := parsedPath.Bucket(), strings.TrimPrefix(parsedPath.Path, "/")

				clusters := map[string]ClusterStatus{}
				for cluster := range knownClusters {
					status := ClusterStatusReachable
					client, ok := r.buildClients[cluster]
					if !ok {
						status = ClusterStatusNoManager
					} else {
						var pods corev1.PodList
						if err := client.List(ctx, &pods, ctrlruntimeclient.MatchingLabels{kube.CreatedByProw: "true"}, ctrlruntimeclient.InNamespace(r.config().PodNamespace), ctrlruntimeclient.Limit(1)); err != nil {
							r.log.WithField("cluster", cluster).WithError(err).Warn("Error listing pod to check for build cluster reachability.")
							status = ClusterStatusUnreachable
						}
					}
					clusters[cluster] = status
				}
				payload, err := json.Marshal(clusters)
				if err != nil {
					r.log.WithError(err).Error("Error marshaling cluster status info.")
					continue
				}
				noCache := "no-cache"
				fullStoragePath, err := providers.StoragePath(bucket, subPath)
				if err != nil {
					r.log.WithError(err).Error("Failed to resolve storage path.")
					continue
				}
				if err := io.WriteContent(ctx, r.log, r.opener, fullStoragePath, payload, io.WriterOptions{CacheControl: &noCache}); err != nil {
					r.log.WithError(err).Error("Error writing cluster status info.")
				}
			}
		}
	}
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	if r.overwriteReconcile != nil {
		return r.overwriteReconcile(ctx, request)
	}
	return r.defaultReconcile(ctx, request)
}

func (r *reconciler) defaultReconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	pj := &prowv1.ProwJob{}
	if err := r.pjClient.Get(ctx, request.NamespacedName, pj); err != nil {
		if !kerrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get prowjob %s: %w", request.Name, err)
		}

		// Objects can be deleted from the API while being in our workqueue
		return reconcile.Result{}, nil
	}

	res, err := r.serializeIfNeeded(ctx, pj)
	if IsTerminalError(err) {
		// Unfixable cases like missing build clusters, do not return an error to prevent requeuing
		r.log.WithError(err).WithFields(pjutil.ProwJobFields(pj)).
			Error("Reconciliation failed with terminal error and will not be requeued")
		return reconcile.Result{}, nil
	}
	if res == nil {
		res = &reconcile.Result{}
	}
	if err != nil {
		r.log.WithError(err).WithField("name", request.Name).Error("Reconciliation failed")
	}
	return *res, err
}

// serializeIfNeeded serializes the reconciliation of Jobs that have a MaxConcurrency setting, otherwise
// multiple reconciliations of the same job may race and not properly respect that setting.
func (r *reconciler) serializeIfNeeded(ctx context.Context, pj *prowv1.ProwJob) (*reconcile.Result, error) {
	if pj.Spec.MaxConcurrency == 0 {
		return r.reconcile(ctx, pj)
	}

	sema := r.serializationLocks.getLock(pj.Spec.Job)
	// Use TryAcquire to avoid blocking workers waiting for the lock
	if !sema.TryAcquire(1) {
		return &reconcile.Result{RequeueAfter: time.Second}, nil
	}
	defer sema.Release(1)
	return r.reconcile(ctx, pj)
}

func (r *reconciler) reconcile(ctx context.Context, pj *prowv1.ProwJob) (*reconcile.Result, error) {
	// terminateDupes first, as that might reduce cluster load and prevent us
	// from doing pointless work.
	if err := r.terminateDupes(ctx, pj); err != nil {
		return nil, fmt.Errorf("terminateDupes failed: %w", err)
	}

	switch pj.Status.State {
	case prowv1.PendingState:
		return r.syncPendingJob(ctx, pj)
	case prowv1.TriggeredState:
		return r.syncTriggeredJob(ctx, pj)
	case prowv1.AbortedState:
		return nil, r.syncAbortedJob(ctx, pj)
	}

	return nil, nil
}

func (r *reconciler) terminateDupes(ctx context.Context, pj *prowv1.ProwJob) error {
	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(ctx, pjs, optPendingTriggeredJobsNamed(pj.Spec.Job)); err != nil {
		return fmt.Errorf("failed to list prowjobs: %w", err)
	}

	return pjutil.TerminateOlderJobs(r.pjClient, r.log, pjs.Items)
}

// syncPendingJob syncs jobs for which we already created the test workload
func (r *reconciler) syncPendingJob(ctx context.Context, pj *prowv1.ProwJob) (*reconcile.Result, error) {
	prevPJ := pj.DeepCopy()

	pod, podExists, err := r.pod(ctx, pj)
	if err != nil {
		return nil, err
	}

	if !podExists {
		// Pod is missing. This can happen in case the previous pod was deleted manually or by
		// a rescheduler. Start a new pod.
		id, pn, err := r.startPod(ctx, pj)
		if err != nil {
			if !isRequestError(err) {
				return nil, fmt.Errorf("error starting pod for PJ %s: %w", pj.Name, err)
			}
			pj.Status.State = prowv1.ErrorState
			pj.SetComplete()
			pj.Status.Description = fmt.Sprintf("Pod can not be created: %v", err)
			r.log.WithFields(pjutil.ProwJobFields(pj)).WithError(err).Warning("Unprocessable pod.")
		} else {
			pj.Status.BuildID = id
			pj.Status.PodName = pn
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pod is missing, starting a new pod")
		}
	} else if pod.Status.Reason == Evicted {
		// Pod was evicted.
		if pj.Spec.ErrorOnEviction {
			// ErrorOnEviction is enabled, complete the PJ and mark it as
			// errored.
			r.log.WithField("error-on-eviction", true).WithFields(pjutil.ProwJobFields(pj)).Info("Pods Node got evicted, fail job.")
			pj.SetComplete()
			pj.Status.State = prowv1.ErrorState
			pj.Status.Description = "Job pod was evicted by the cluster."
		} else {
			// ErrorOnEviction is disabled. Delete the pod now and recreate it in
			// the next resync.
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pods Node got evicted, deleting & next sync loop will restart pod")
			client, ok := r.buildClients[pj.ClusterAlias()]
			if !ok {
				return nil, fmt.Errorf("evicted pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}
			if finalizers := sets.NewString(pod.Finalizers...); finalizers.Has(kubernetesreporterapi.FinalizerName) {
				// We want the end user to not see this, so we have to remove the finalizer, otherwise the pod hangs
				oldPod := pod.DeepCopy()
				pod.Finalizers = finalizers.Delete(kubernetesreporterapi.FinalizerName).UnsortedList()
				if err := client.Patch(ctx, pod, ctrlruntimeclient.MergeFrom(oldPod)); err != nil {
					return nil, fmt.Errorf("failed to patch pod trying to remove %s finalizer: %w", kubernetesreporterapi.FinalizerName, err)
				}
			}
			r.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
			return nil, ctrlruntimeclient.IgnoreNotFound(client.Delete(ctx, pod))
		}
	} else if pod.DeletionTimestamp != nil && pod.Status.Reason == NodeUnreachablePodReason {
		// This can happen in any phase and means the node got evicted after it became unresponsive. Delete the finalizer so the pod
		// vanishes and we will silently re-create it in the next iteration.
		r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pods Node got lost, deleting & next sync loop will restart pod")
		client, ok := r.buildClients[pj.ClusterAlias()]
		if !ok {
			return nil, fmt.Errorf("unknown pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
		}

		if finalizers := sets.NewString(pod.Finalizers...); finalizers.Has(kubernetesreporterapi.FinalizerName) {
			// We want the end user to not see this, so we have to remove the finalizer, otherwise the pod hangs
			oldPod := pod.DeepCopy()
			pod.Finalizers = finalizers.Delete(kubernetesreporterapi.FinalizerName).UnsortedList()
			if err := client.Patch(ctx, pod, ctrlruntimeclient.MergeFrom(oldPod)); err != nil {
				return nil, fmt.Errorf("failed to patch pod trying to remove %s finalizer: %w", kubernetesreporterapi.FinalizerName, err)
			}
		}

		return nil, nil
	} else {
		switch pod.Status.Phase {
		case corev1.PodUnknown:
			// Pod is in Unknown state. This can happen if there is a problem with
			// the node. Delete the old pod, this will fire an event that triggers
			// a new reconciliation in which we will re-create the pod.
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pod is in unknown state, deleting & restarting pod")
			client, ok := r.buildClients[pj.ClusterAlias()]
			if !ok {
				return nil, fmt.Errorf("unknown pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}

			if finalizers := sets.NewString(pod.Finalizers...); finalizers.Has(kubernetesreporterapi.FinalizerName) {
				// We want the end user to not see this, so we have to remove the finalizer, otherwise the pod hangs
				oldPod := pod.DeepCopy()
				pod.Finalizers = finalizers.Delete(kubernetesreporterapi.FinalizerName).UnsortedList()
				if err := client.Patch(ctx, pod, ctrlruntimeclient.MergeFrom(oldPod)); err != nil {
					return nil, fmt.Errorf("failed to patch pod trying to remove %s finalizer: %w", kubernetesreporterapi.FinalizerName, err)
				}
			}
			r.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
			return nil, ctrlruntimeclient.IgnoreNotFound(client.Delete(ctx, pod))

		case corev1.PodSucceeded:
			pj.SetComplete()
			// There were bugs around this in the past so be paranoid and verify each container
			// https://github.com/kubernetes/kubernetes/issues/58711 is only fixed in 1.18+
			if didPodSucceed(pod) {
				// Pod succeeded. Update ProwJob and talk to GitHub.
				pj.Status.State = prowv1.SuccessState
				pj.Status.Description = "Job succeeded."
			} else {
				pj.Status.State = prowv1.ErrorState
				pj.Status.Description = "Pod was in succeeded phase but some containers didn't finish"
			}

		case corev1.PodFailed:
			// Pod failed. Update ProwJob, talk to GitHub.
			pj.SetComplete()
			pj.Status.State = prowv1.FailureState
			pj.Status.Description = "Job failed."

		case corev1.PodPending:
			var requeueAfter time.Duration
			maxPodPending := r.config().Plank.PodPendingTimeout.Duration
			if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.PodPendingTimeout != nil {
				maxPodPending = pj.Spec.DecorationConfig.PodPendingTimeout.Duration
			}
			maxPodUnscheduled := r.config().Plank.PodUnscheduledTimeout.Duration
			if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.PodUnscheduledTimeout != nil {
				maxPodUnscheduled = pj.Spec.DecorationConfig.PodUnscheduledTimeout.Duration
			}
			if pod.Status.StartTime.IsZero() {
				if time.Since(pod.CreationTimestamp.Time) >= maxPodUnscheduled {
					// Pod is stuck in unscheduled state longer than maxPodUncheduled
					// abort the job, and talk to GitHub
					pj.SetComplete()
					pj.Status.State = prowv1.ErrorState
					pj.Status.Description = "Pod scheduling timeout."
					r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Marked job for stale unscheduled pod as errored.")
					if err := r.deletePod(ctx, pj); err != nil {
						return nil, fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", pod.Namespace, pod.Name, pj.ClusterAlias(), err)
					}
					break
				} else {
					// We have to re-check on the pod once we reached maxPodUnscheduled to
					// be able to fail the job if it didn't get scheduled by then.
					requeueAfter = maxPodUnscheduled - time.Since(pod.CreationTimestamp.Time)
				}
			} else {
				if time.Since(pod.Status.StartTime.Time) >= maxPodPending {
					// Pod is stuck in pending state longer than maxPodPending
					// abort the job, and talk to GitHub
					pj.SetComplete()
					pj.Status.State = prowv1.ErrorState
					pj.Status.Description = "Pod pending timeout."
					r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Marked job for stale pending pod as errored.")
					if err := r.deletePod(ctx, pj); err != nil {
						return nil, fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", pod.Namespace, pod.Name, pj.ClusterAlias(), err)
					}
					break
				} else {
					// We have to re-check on the pod once we reached maxPodPending to
					// be able to fail the job if it didn't start running by then.
					requeueAfter = maxPodPending - time.Since(pod.Status.StartTime.Time)
				}
			}
			// Pod didn't start but didn't reach the scheduling or pending timeout yet,
			// do nothing but check on it again once the timeout is reached.
			if pod.DeletionTimestamp == nil {
				return &reconcile.Result{RequeueAfter: requeueAfter}, nil
			}
		case corev1.PodRunning:
			if pod.DeletionTimestamp != nil {
				break
			}
			maxPodRunning := r.config().Plank.PodRunningTimeout.Duration
			if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.PodRunningTimeout != nil {
				maxPodRunning = pj.Spec.DecorationConfig.PodRunningTimeout.Duration
			}
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodRunning {
				// Pod is still running. Do nothing.
				return nil, nil
			}

			// Pod is stuck in running state longer than maxPodRunning
			// abort the job, and talk to GitHub
			pj.SetComplete()
			pj.Status.State = prowv1.AbortedState
			pj.Status.Description = "Pod running timeout."
			if err := r.deletePod(ctx, pj); err != nil {
				return nil, fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", pod.Namespace, pod.Name, pj.ClusterAlias(), err)
			}
		default:
			if pod.DeletionTimestamp == nil {
				// other states, ignore
				return nil, nil
			}
		}
	}

	// If a pod gets deleted unexpectedly, it might be in any phase and will stick around until
	// we complete the job if the kubernetes reporter is used, because it sets a finalizer.
	if !pj.Complete() && pod != nil && pod.DeletionTimestamp != nil {
		pj.SetComplete()
		pj.Status.State = prowv1.ErrorState
		pj.Status.Description = "Pod got deleted unexpectedly"
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

	if err := r.pjClient.Patch(ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
		return nil, fmt.Errorf("patching prowjob: %w", err)
	}

	// If the ProwJob state has changed, we must ensure that the update reaches the cache before
	// processing the key again. Without this we might accidentally replace intentionally deleted pods
	// or otherwise incorrectly react to stale ProwJob state.
	state := pj.Status.State
	if prevPJ.Status.State == state {
		return nil, nil
	}
	nn := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
	if err := wait.Poll(100*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := r.pjClient.Get(ctx, nn, pj); err != nil {
			return false, fmt.Errorf("failed to get prowjob: %w", err)
		}
		return pj.Status.State == state, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for cached prowjob %s to get into state %s: %w", nn.String(), state, err)
	}

	return nil, nil
}

// syncTriggeredJob syncs jobs that do not yet have an associated test workload running
func (r *reconciler) syncTriggeredJob(ctx context.Context, pj *prowv1.ProwJob) (*reconcile.Result, error) {
	prevPJ := pj.DeepCopy()

	var id, pn string

	pod, podExists, err := r.pod(ctx, pj)
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
		canExecuteConcurrently, err := r.canExecuteConcurrently(ctx, pj)
		if err != nil {
			return nil, fmt.Errorf("canExecuteConcurrently: %w", err)
		}
		if !canExecuteConcurrently {
			return &reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// We haven't started the pod yet. Do so.
		id, pn, err = r.startPod(ctx, pj)
		if err != nil {
			if !isRequestError(err) {
				return nil, fmt.Errorf("error starting pod: %w", err)
			}
			pj.Status.State = prowv1.ErrorState
			pj.SetComplete()
			pj.Status.Description = fmt.Sprintf("Pod can not be created: %v", err)
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
	if err := r.pjClient.Patch(ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
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
		if err := r.pjClient.Get(ctx, nn, pj); err != nil {
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
func (r *reconciler) syncAbortedJob(ctx context.Context, pj *prowv1.ProwJob) error {

	buildClient, ok := r.buildClients[pj.ClusterAlias()]
	if !ok {
		return fmt.Errorf("no build client available for cluster %s", pj.ClusterAlias())
	}

	// Just optimistically delete and swallow the potential 404
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      pj.Name,
		Namespace: r.config().PodNamespace,
	}}
	if err := ctrlruntimeclient.IgnoreNotFound(buildClient.Delete(ctx, pod)); err != nil {
		return fmt.Errorf("failed to delete pod %s/%s in cluster %s: %w", pod.Namespace, pod.Name, pj.ClusterAlias(), err)
	}

	originalPJ := pj.DeepCopy()
	pj.SetComplete()
	return r.pjClient.Patch(ctx, pj, ctrlruntimeclient.MergeFrom(originalPJ))
}

// pod Gets pod for a pj, returns pod, whether pod exist, and error.
func (r *reconciler) pod(ctx context.Context, pj *prowv1.ProwJob) (*corev1.Pod, bool, error) {
	buildClient, buildClientExists := r.buildClients[pj.ClusterAlias()]
	if !buildClientExists {
		return nil, false, TerminalError(fmt.Errorf("no build client found for cluster %q", pj.ClusterAlias()))
	}

	pod := &corev1.Pod{}
	name := types.NamespacedName{
		Namespace: r.config().PodNamespace,
		Name:      pj.Name,
	}

	if err := buildClient.Get(ctx, name, pod); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get pod: %w", err)
	}

	return pod, true, nil
}

func (r *reconciler) deletePod(ctx context.Context, pj *prowv1.ProwJob) error {
	buildClient, buildClientExists := r.buildClients[pj.ClusterAlias()]
	if !buildClientExists {
		return TerminalError(fmt.Errorf("no build client found for cluster %q", pj.ClusterAlias()))
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.config().PodNamespace,
			Name:      pj.Name,
		},
	}

	if err := ctrlruntimeclient.IgnoreNotFound(buildClient.Delete(ctx, pod)); err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Deleted stale running pod.")
	return nil
}

func (r *reconciler) startPod(ctx context.Context, pj *prowv1.ProwJob) (string, string, error) {
	buildID, err := r.getBuildID(pj.Spec.Job)
	if err != nil {
		return "", "", fmt.Errorf("error getting build ID: %w", err)
	}

	pj.Status.BuildID = buildID
	pod, err := decorate.ProwJobToPod(*pj)
	if err != nil {
		return "", "", err
	}
	pod.Namespace = r.config().PodNamespace
	// Add prow version as a label for better debugging prowjobs.
	pod.ObjectMeta.Labels[kube.PlankVersionLabel] = version.Version
	podName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}

	client, ok := r.buildClients[pj.ClusterAlias()]
	if !ok {
		return "", "", TerminalError(fmt.Errorf("unknown cluster alias %q", pj.ClusterAlias()))
	}
	err = client.Create(ctx, pod)
	r.log.WithFields(pjutil.ProwJobFields(pj)).Debug("Create Pod.")
	if err != nil {
		return "", "", fmt.Errorf("create pod %s in cluster %s: %w", podName.String(), pj.ClusterAlias(), err)
	}

	// We must block until we see the pod, otherwise a new reconciliation may be triggered that tries to create
	// the pod because its not in the cache yet, errors with IsAlreadyExists and sets the prowjob to failed
	if err := wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		if err := client.Get(ctx, podName, pod); err != nil {
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
func (r *reconciler) canExecuteConcurrently(ctx context.Context, pj *prowv1.ProwJob) (bool, error) {

	if max := r.config().Plank.MaxConcurrency; max > 0 {
		pjs := &prowv1.ProwJobList{}
		if err := r.pjClient.List(ctx, pjs, optPendingProwJobs()); err != nil {
			return false, fmt.Errorf("failed to list prowjobs: %w", err)
		}
		// The list contains our own ProwJob
		running := len(pjs.Items) - 1
		if running >= max {
			r.log.WithFields(pjutil.ProwJobFields(pj)).Infof("Not starting another job, already %d running.", running)
			return false, nil
		}
	}

	if canExecute, err := r.canExecuteConcurrentlyPerJob(ctx, pj); err != nil || !canExecute {
		return canExecute, err
	}

	return r.canExecuteConcurrentlyPerQueue(ctx, pj)
}

func (r *reconciler) canExecuteConcurrentlyPerJob(ctx context.Context, pj *prowv1.ProwJob) (bool, error) {
	if pj.Spec.MaxConcurrency == 0 {
		return true, nil
	}

	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(ctx, pjs, optPendingTriggeredJobsNamed(pj.Spec.Job)); err != nil {
		return false, fmt.Errorf("failed listing prowjobs: %w:", err)
	}
	r.log.Infof("got %d not completed with same name", len(pjs.Items))

	pendingOrOlderMatchingPJs := countPendingOrOlderTriggeredMatchingPJs(*pj, pjs.Items)
	if pendingOrOlderMatchingPJs >= pj.Spec.MaxConcurrency {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			Debugf("Not starting another instance of %s, have %d instances that are pending or older, %d is the limit",
				pj.Spec.Job, pendingOrOlderMatchingPJs, pj.Spec.MaxConcurrency)
		return false, nil
	}

	return true, nil
}

func (r *reconciler) canExecuteConcurrentlyPerQueue(ctx context.Context, pj *prowv1.ProwJob) (bool, error) {
	queueName := pj.Spec.JobQueueName
	if queueName == "" {
		return true, nil
	}

	queueConcurrency, queueDefined := r.config().Plank.JobQueueCapacities[queueName]
	if !queueDefined {
		return false, fmt.Errorf("failed to match queue name '%s' with Plank configuration", queueName)
	}
	if queueConcurrency == 0 {
		return false, nil
	}
	if queueConcurrency < 0 {
		return true, nil
	}

	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(ctx, pjs, optPendingTriggeredJobsInQueue(queueName)); err != nil {
		return false, fmt.Errorf("failed listing prowjobs in queue %s: %w", queueName, err)
	}
	r.log.Infof("got %d not completed within queue %s", len(pjs.Items), queueName)

	pendingOrOlderMatchingPJs := countPendingOrOlderTriggeredMatchingPJs(*pj, pjs.Items)
	if pendingOrOlderMatchingPJs >= queueConcurrency {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			Debugf("Not starting another instance of %s, have %d instances in queue %s that are pending or older, %d is the limit",
				pj.Spec.Job, pendingOrOlderMatchingPJs, queueName, queueConcurrency)
		return false, nil
	}

	return true, nil
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

	return predicate.NewPredicateFuncs(func(o ctrlruntimeclient.Object) bool {
		result := func() bool {
			pj, ok := o.(*prowv1.ProwJob)
			if !ok {
				// We ignore pods that do not match our selector
				return selector.Matches(labels.Set(o.GetLabels()))
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
	return handler.EnqueueRequestsFromMapFunc(func(o ctrlruntimeclient.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: ctrlruntimeclient.ObjectKey{
			Namespace: prowJobNamespace,
			Name:      o.GetName(),
		}}}
	})
}

const (
	// prowJobIndexName is the name of an index that
	// holds all ProwJobs that are in the correct namespace
	// and use the Kubernetes agent
	prowJobIndexName = "plank-prow-jobs"
	// prowJobIndexKeyAll is the indexKey for all ProwJobs
	prowJobIndexKeyAll = "all"
	// prowJobIndexKeyPending is the indexKey for prowjobs
	// that are currently pending AKA a corresponding pod
	// exists but didn't yet finish
	prowJobIndexKeyPending = "pending"
)

func pendingTriggeredIndexKeyByName(jobName string) string {
	return fmt.Sprintf("pending-triggered-named-%s", jobName)
}

func pendingTriggeredIndexKeyByJobQueueName(jobQueueName string) string {
	return fmt.Sprintf("pending-triggered-with-job-queue-name-%s", jobQueueName)
}

func prowJobIndexer(prowJobNamespace string) ctrlruntimeclient.IndexerFunc {
	return func(o ctrlruntimeclient.Object) []string {
		pj := o.(*prowv1.ProwJob)
		if pj.Namespace != prowJobNamespace || pj.Spec.Agent != prowv1.KubernetesAgent {
			return nil
		}

		indexes := []string{prowJobIndexKeyAll}

		if pj.Status.State == prowv1.PendingState {
			indexes = append(indexes, prowJobIndexKeyPending)
		}

		if pj.Status.State == prowv1.PendingState || pj.Status.State == prowv1.TriggeredState {
			indexes = append(indexes, pendingTriggeredIndexKeyByName(pj.Spec.Job))

			if pj.Spec.JobQueueName != "" {
				indexes = append(indexes, pendingTriggeredIndexKeyByJobQueueName(pj.Spec.JobQueueName))
			}
		}

		return indexes
	}
}

func optAllProwJobs() ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingFields{prowJobIndexName: prowJobIndexKeyAll}
}

func optPendingProwJobs() ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingFields{prowJobIndexName: prowJobIndexKeyPending}
}

func optPendingTriggeredJobsNamed(name string) ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingFields{prowJobIndexName: pendingTriggeredIndexKeyByName(name)}
}

func optPendingTriggeredJobsInQueue(queueName string) ctrlruntimeclient.ListOption {
	return ctrlruntimeclient.MatchingFields{prowJobIndexName: pendingTriggeredIndexKeyByJobQueueName(queueName)}
}

func didPodSucceed(p *corev1.Pod) bool {
	if p.Status.Phase != corev1.PodSucceeded {
		return false
	}
	for _, container := range append(p.Status.ContainerStatuses, p.Status.InitContainerStatuses...) {
		if container.State.Terminated == nil || container.State.Terminated.ExitCode != 0 || container.State.Terminated.FinishedAt.IsZero() {
			return false
		}
	}

	return true
}

func getPodBuildID(pod *corev1.Pod) string {
	if buildID, ok := pod.ObjectMeta.Labels[kube.ProwBuildIDLabel]; ok && buildID != "" {
		return buildID
	}

	// For backwards compatibility: existing pods may not have the buildID label.
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "BUILD_ID" {
			return env.Value
		}
	}

	logrus.Warningf("BUILD_ID was not found in pod %q: streaming logs from deck will not work", pod.ObjectMeta.Name)
	return ""
}

// isRequestError extracts an HTTP status code from a kerrors.APIStatus and
// returns true if it is a 4xx error.
func isRequestError(err error) bool {
	var code int32 = 500 // This is what kerrors.ReasonForError() defaults to.
	if status := kerrors.APIStatus(nil); errors.As(err, &status) {
		code = status.Status().Code
	}
	return 400 <= code && code < 500
}

func countPendingOrOlderTriggeredMatchingPJs(pj prowv1.ProwJob, pjs []prowv1.ProwJob) int {
	var pendingOrOlderTriggeredMatchingPJs int

	for _, foundPJ := range pjs {
		// Ignore self here.
		if foundPJ.UID == pj.UID {
			continue
		}
		if foundPJ.Status.State == prowv1.PendingState {
			pendingOrOlderTriggeredMatchingPJs++
			continue
		}

		// At this point if foundPJ is older than our prowJobs it gets
		// priorized to make sure we execute jobs in creation order.
		if foundPJ.Status.State == prowv1.TriggeredState &&
			foundPJ.CreationTimestamp.Before(&pj.CreationTimestamp) {
			pendingOrOlderTriggeredMatchingPJs++
		}
	}

	return pendingOrOlderTriggeredMatchingPJs
}
