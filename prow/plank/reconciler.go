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
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

type reconciler struct {
	ctx          context.Context
	pjClient     ctrlruntimeclient.Client
	buildClients map[string]ctrlruntimeclient.Client
	log          *logrus.Entry
	config       config.Getter
	totURL       string
	lock         sync.RWMutex
	clock        clock.Clock

	// pendingJobs is a short-lived cache that helps in limiting
	// the maximum concurrency of jobs.
	pendingJobs map[string]int
}

// TODO: Predicate for createdByProw: true && c.selector, if defined && pj.Spec.Agent == kubernetes
// TODO: Metrics

func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	res, err := r.reconcile(pj)
	if res == nil {
		res = &reconcile.Result{}
	}
	if err != nil {
		r.log.WithError(err).Error("Reconciliation failed")
	}
	return *res, err
}

func (r *reconciler) reconcile(pj *prowv1.ProwJob) (*reconcile.Result, error) {
	switch pj.Status.State {
	case prowv1.PendingState:
		return nil, r.syncPendingJob(pj)
	case prowv1.TriggeredState:
		return r.syncTriggeredJob(pj)
	}

	return nil, nil
}

func (r *reconciler) teminateDupes(pj *prowv1.ProwJob) error {
	// TODO: This is incredibly inefficient, we list all prowjobs on each sync.
	// This must be replaced with an index over jobName + !completed
	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(r.ctx, pjs, ctrlruntimeclient.InNamespace(r.config().ProwJobNamespace)); err != nil {
		return fmt.Errorf("failed to list prowjobs: %v", err)
	}

	return pjutil.TerminateOlderJobs(r.pjClient, r.log, pjs.Items, func(toCancel prowv1.ProwJob) error {
		client, ok := r.buildClients[pj.ClusterAlias()]
		if !ok {
			return fmt.Errorf("no client for cluster %q present", pj.ClusterAlias())
		}
		podToDelete := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.config().PodNamespace,
				Name:      toCancel.Name,
			},
		}
		if err := client.Delete(r.ctx, podToDelete); err != nil {
			return fmt.Errorf("failed to delete pod %s/%s: %w", podToDelete.Namespace, podToDelete.Name, err)
		}
		return nil
	})
}

func (r *reconciler) syncPendingJob(pj *prowv1.ProwJob) error {
	prevPJ := pj.DeepCopy()

	pod, podExists, err := r.pod(pj)
	if err != nil {
		return err
	}

	if !podExists {
		r.incrementNumPendingJobs(pj.Spec.Job)
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
			r.incrementNumPendingJobs(pj.Spec.Job)
			// Pod is in Unknown state. This can happen if there is a problem with
			// the node. Delete the old pod, we'll start a new one next loop.
			r.log.WithFields(pjutil.ProwJobFields(pj)).Info("Pod is in unknown state, deleting & restarting pod")
			client, ok := r.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("unknown pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}

			r.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
			return client.Delete(r.ctx, pod)

		case corev1.PodSucceeded:
			// Pod succeeded. Update ProwJob, talk to GitHub, and start next jobs.
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
				r.incrementNumPendingJobs(pj.Spec.Job)
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
			r.incrementNumPendingJobs(pj.Spec.Job)
			return nil
		case corev1.PodRunning:
			maxPodRunning := r.config().Plank.PodRunningTimeout.Duration
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodRunning {
				// Pod is still running. Do nothing.
				r.incrementNumPendingJobs(pj.Spec.Job)
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
			r.incrementNumPendingJobs(pj.Spec.Job)
			return nil
		}
	}

	pj.Status.URL = pjutil.JobURL(r.config().Plank, *pj, r.log)

	if prevPJ.Status.State != pj.Status.State {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}

	if err := r.pjClient.Patch(r.ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
		return fmt.Errorf("patching prowjob: %v", err)
	}

	return nil
}

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
	if !podExists {
		// Do not start more jobs than specified and check again later.
		canExecuteConcurrently, err := r.canExecuteConcurrently(pj)
		if err != nil {
			return nil, fmt.Errorf("canExecuteConcurrently: %v", err)
		}
		if !canExecuteConcurrently {
			return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
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
	} else {
		id = getPodBuildID(pod)
		pn = pod.ObjectMeta.Name
	}

	if pj.Status.State == prowv1.TriggeredState {
		// BuildID needs to be set before we execute the job url template.
		pj.Status.BuildID = id
		now := metav1.NewTime(r.clock.Now())
		pj.Status.PendingTime = &now
		pj.Status.State = prowv1.PendingState
		pj.Status.PodName = pn
		pj.Status.Description = "Job triggered."
		pj.Status.URL = pjutil.JobURL(r.config().Plank, *pj, r.log)
	}

	if prevPJ.Status.State != pj.Status.State {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	return nil, r.pjClient.Patch(r.ctx, pj.DeepCopy(), ctrlruntimeclient.MergeFrom(prevPJ))
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

// incrementNumPendingJobs increments the amount of
// pending ProwJobs for the given job identifier
func (r *reconciler) incrementNumPendingJobs(job string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.pendingJobs[job]++
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
	return buildID, pod.Name, nil
}

func (r *reconciler) getBuildID(name string) (string, error) {
	return pjutil.GetBuildID(name, r.totURL)
}

func (r *reconciler) canExecuteConcurrently(pj *prowv1.ProwJob) (bool, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if max := r.config().Plank.MaxConcurrency; max > 0 {
		var running int
		for _, num := range r.pendingJobs {
			running += num
		}
		if running >= max {
			r.log.WithFields(pjutil.ProwJobFields(pj)).Debugf("Not starting another job, already %d running.", running)
			return false, nil
		}
	}

	if pj.Spec.MaxConcurrency == 0 {
		r.pendingJobs[pj.Spec.Job]++
		return true, nil
	}

	numPending := r.pendingJobs[pj.Spec.Job]
	if numPending >= pj.Spec.MaxConcurrency {
		r.log.WithFields(pjutil.ProwJobFields(pj)).Debugf("Not starting another instance of %s, already %d running.", pj.Spec.Job, numPending)
		return false, nil
	}

	var olderMatchingPJs int

	// TODO: This is incredible inefficient, as we we list all prowJobs everytime we do a sync.
	// It must be replaced with an index over .Spec.Job + .Status.State
	pjs := &prowv1.ProwJobList{}
	if err := r.pjClient.List(r.ctx, pjs, ctrlruntimeclient.InNamespace(r.config().ProwJobNamespace)); err != nil {
		return false, fmt.Errorf("list prowjobs: %v", err)
	}

	for _, foundPJ := range pjs.Items {
		if foundPJ.Status.State != prowv1.TriggeredState {
			continue
		}
		if foundPJ.Spec.Job != pj.Spec.Job {
			continue
		}
		if foundPJ.CreationTimestamp.Before(&pj.CreationTimestamp) {
			olderMatchingPJs++
		}
	}

	if numPending+olderMatchingPJs >= pj.Spec.MaxConcurrency {
		r.log.WithFields(pjutil.ProwJobFields(pj)).
			Debugf("Not starting another instance of %s, already %d running and %d older instances waiting, together they equal or exceed the total limit of %d",
				pj.Spec.Job, numPending, olderMatchingPJs, pj.Spec.MaxConcurrency)
		return false, nil
	}

	r.pendingJobs[pj.Spec.Job]++
	return true, nil
}
