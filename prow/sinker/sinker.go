/*
Copyright 2019 The Kubernetes Authors.

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

package sinker

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

func init() {
	prometheus.MustRegister(sinkerMetrics.podsCreated)
	prometheus.MustRegister(sinkerMetrics.timeUsed)
	prometheus.MustRegister(sinkerMetrics.podsRemoved)
	prometheus.MustRegister(sinkerMetrics.podRemovalErrors)
	prometheus.MustRegister(sinkerMetrics.prowJobsCreated)
	prometheus.MustRegister(sinkerMetrics.prowJobsCleaned)
	prometheus.MustRegister(sinkerMetrics.prowJobsCleaningErrors)
}

const (
	controllerName = "sinker"

	reasonPodAged     = "aged"
	reasonPodOrphaned = "orphaned"

	reasonProwJobAged         = "aged"
	reasonProwJobAgedPeriodic = "aged-periodic"
)

// Prometheus Metrics
var (
	sinkerMetrics = struct {
		podsCreated            prometheus.Gauge
		timeUsed               prometheus.Gauge
		podsRemoved            *prometheus.GaugeVec
		podRemovalErrors       *prometheus.GaugeVec
		prowJobsCreated        prometheus.Gauge
		prowJobsCleaned        *prometheus.GaugeVec
		prowJobsCleaningErrors *prometheus.GaugeVec
	}{
		podsCreated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sinker_pods_existing",
			Help: "Number of the existing pods in each sinker cleaning.",
		}),
		timeUsed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sinker_loop_duration_seconds",
			Help: "Time used in each sinker cleaning.",
		}),
		podsRemoved: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sinker_pods_removed",
			Help: "Number of pods removed in each sinker cleaning.",
		}, []string{
			"reason",
		}),
		podRemovalErrors: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sinker_pod_removal_errors",
			Help: "Number of errors which occurred in each sinker pod cleaning.",
		}, []string{
			"reason",
		}),
		prowJobsCreated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sinker_prow_jobs_existing",
			Help: "Number of the existing prow jobs in each sinker cleaning.",
		}),
		prowJobsCleaned: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sinker_prow_jobs_cleaned",
			Help: "Number of prow jobs cleaned in each sinker cleaning.",
		}, []string{
			"reason",
		}),
		prowJobsCleaningErrors: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sinker_prow_jobs_cleaning_errors",
			Help: "Number of errors which occurred in each sinker prow job cleaning.",
		}, []string{
			"reason",
		}),
	}
)

type controller struct {
	logger               *logrus.Entry
	controlClusterClient ctrlruntimeclient.Client
	podClients           []corev1.PodInterface
	cfg                  config.Getter
	client               ctrlruntimeclient.Client
	interval             time.Duration
	runOnce              bool
	dryRun               bool
}

type sinkerReconciliationMetrics struct {
	podsCreated            int
	startAt                time.Time
	finishedAt             time.Time
	podsRemoved            map[string]int
	podRemovalErrors       map[string]int
	prowJobsCreated        int
	prowJobsCleaned        map[string]int
	prowJobsCleaningErrors map[string]int
}

func (m *sinkerReconciliationMetrics) getPodsTotalRemoved() int {
	result := 0
	for _, v := range m.podsRemoved {
		result += v
	}
	return result
}

func (m *sinkerReconciliationMetrics) getTimeUsed() time.Duration {
	return m.finishedAt.Sub(m.startAt)
}

// Add creates a new sinker controller and adds it to the passed in manager.Manager
func Add(mgr manager.Manager, logger *logrus.Entry, podClients []corev1.PodInterface, cfg config.Getter, interval time.Duration, runOnce, dryRun bool) error {
	c := &controller{
		logger:               logger,
		controlClusterClient: mgr.GetClient(),
		podClients:           podClients,
		cfg:                  cfg,
		interval:             interval,
		runOnce:              runOnce,
		dryRun:               dryRun,
	}

	return mgr.Add(c)
}

func (c *controller) Start(stopChan <-chan struct{}) error {

	ticker := time.NewTicker(c.interval)
	for {
		select {
		case <-stopChan:
			c.logger.Info("Stop signal received, exiting")
			ticker.Stop()
			return nil
		case <-ticker.C:
			start := time.Now()
			c.clean()
			c.logger.Infof("Sync time: %v", time.Since(start))
			if c.runOnce {
				c.logger.Info("Run once mode enabled, exiting")
				return nil
			}
		}
	}
}

func (c *controller) clean() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metrics := sinkerReconciliationMetrics{
		startAt:                time.Now(),
		podsRemoved:            map[string]int{},
		podRemovalErrors:       map[string]int{},
		prowJobsCleaned:        map[string]int{},
		prowJobsCleaningErrors: map[string]int{}}

	// Clean up old prow jobs first.
	prowJobs := &prowapi.ProwJobList{}
	if err := c.controlClusterClient.List(ctx, &ctrlruntimeclient.ListOptions{}, prowJobs); err != nil {
		c.logger.WithError(err).Error("Error listing prow jobs.")
		return
	}
	metrics.prowJobsCreated = len(prowJobs.Items)

	// Only delete pod if its prowjob is marked as finished
	isExist := sets.NewString()
	isFinished := sets.NewString()

	maxProwJobAge := c.cfg().Sinker.MaxProwJobAge.Duration
	for _, prowJob := range prowJobs.Items {
		isExist.Insert(prowJob.ObjectMeta.Name)
		// Handle periodics separately.
		if prowJob.Spec.Type == prowapi.PeriodicJob {
			continue
		}
		if !prowJob.Complete() {
			continue
		}
		isFinished.Insert(prowJob.ObjectMeta.Name)
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if c.dryRun {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Omitting deletion of prowjob as dry-run is enabled")
			continue
		}
		if err := c.controlClusterClient.Delete(ctx, &prowJob); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
			metrics.prowJobsCleaned[reasonProwJobAged]++
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
			metrics.prowJobsCleaningErrors[string(k8serrors.ReasonForError(err))]++
		}
	}

	// Keep track of what periodic jobs are in the config so we will
	// not clean up their last prowjob.
	isActivePeriodic := make(map[string]bool)
	for _, p := range c.cfg().Periodics {
		isActivePeriodic[p.Name] = true
	}

	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestProwJobs(prowJobs.Items, prowapi.PeriodicJob)
	for _, prowJob := range prowJobs.Items {
		if prowJob.Spec.Type != prowapi.PeriodicJob {
			continue
		}

		latestPJ := latestPeriodics[prowJob.Spec.Job]
		if isActivePeriodic[prowJob.Spec.Job] && prowJob.ObjectMeta.Name == latestPJ.ObjectMeta.Name {
			// Ignore deleting this one.
			continue
		}
		if !prowJob.Complete() {
			continue
		}
		isFinished.Insert(prowJob.ObjectMeta.Name)
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if c.dryRun {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Omitting deletion of prowjob as dry-run is enabled")
			continue
		}
		if err := c.controlClusterClient.Delete(ctx, &prowJob); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
			metrics.prowJobsCleaned[reasonProwJobAgedPeriodic]++
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
			metrics.prowJobsCleaningErrors[string(k8serrors.ReasonForError(err))]++
		}
	}

	// Now clean up old pods.
	selector := fmt.Sprintf("%s = %s", kube.CreatedByProw, "true")
	for _, client := range c.podClients {
		pods, err := client.List(metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			c.logger.WithError(err).Error("Error listing pods.")
			return
		}
		metrics.podsCreated += len(pods.Items)
		maxPodAge := c.cfg().Sinker.MaxPodAge.Duration
		for _, pod := range pods.Items {
			clean := !pod.Status.StartTime.IsZero() && time.Since(pod.Status.StartTime.Time) > maxPodAge
			reason := reasonPodAged
			if !isFinished.Has(pod.ObjectMeta.Name) {
				// prowjob exists and is not marked as completed yet
				// deleting the pod now will result in plank creating a brand new pod
				clean = false
			}
			if !isExist.Has(pod.ObjectMeta.Name) {
				// prowjob has gone, we want to clean orphan pods regardless of the state
				reason = reasonPodOrphaned
				clean = true
			}

			if !clean {
				continue
			}

			// Delete old finished or orphan pods. Don't quit if we fail to delete one.
			if err := client.Delete(pod.ObjectMeta.Name, &metav1.DeleteOptions{}); err == nil {
				c.logger.WithField("pod", pod.ObjectMeta.Name).Info("Deleted old completed pod.")
				metrics.podsRemoved[reason]++
			} else {
				c.logger.WithField("pod", pod.ObjectMeta.Name).WithError(err).Error("Error deleting pod.")
				metrics.podRemovalErrors[string(k8serrors.ReasonForError(err))]++
			}
		}
	}

	metrics.finishedAt = time.Now()
	sinkerMetrics.podsCreated.Set(float64(metrics.podsCreated))
	sinkerMetrics.timeUsed.Set(float64(metrics.getTimeUsed().Seconds()))
	for k, v := range metrics.podsRemoved {
		sinkerMetrics.podsRemoved.WithLabelValues(k).Set(float64(v))
	}
	for k, v := range metrics.podRemovalErrors {
		sinkerMetrics.podRemovalErrors.WithLabelValues(k).Set(float64(v))
	}
	sinkerMetrics.prowJobsCreated.Set(float64(metrics.prowJobsCreated))
	for k, v := range metrics.prowJobsCleaned {
		sinkerMetrics.prowJobsCleaned.WithLabelValues(k).Set(float64(v))
	}
	for k, v := range metrics.prowJobsCleaningErrors {
		sinkerMetrics.prowJobsCleaningErrors.WithLabelValues(k).Set(float64(v))
	}
	c.logger.Info("Sinker reconciliation complete.")
}
