/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pjutil/pprof"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	kubernetesreporterapi "k8s.io/test-infra/prow/crier/reporters/gcs/kubernetes/api"
	"k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
	_ "k8s.io/test-infra/prow/version"
)

type options struct {
	runOnce                bool
	config                 configflagutil.ConfigOptions
	dryRun                 bool
	kubernetes             flagutil.KubernetesOptions
	instrumentationOptions flagutil.InstrumentationOptions
}

const (
	reasonPodAged     = "aged"
	reasonPodOrphaned = "orphaned"
	reasonPodTTLed    = "ttled"

	reasonProwJobAged         = "aged"
	reasonProwJobAgedPeriodic = "aged-periodic"
)

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	o := options{}
	fs.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to Kubernetes.")

	o.config.AddFlags(fs)
	o.kubernetes.AddFlags(fs)
	o.instrumentationOptions.AddFlags(fs)
	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	if err := o.kubernetes.Validate(o.dryRun); err != nil {
		return err
	}

	if err := o.config.Validate(o.dryRun); err != nil {
		return err
	}

	return nil
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pprof.Instrument(o.instrumentationOptions)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config
	o.kubernetes.SetDisabledClusters(sets.New[string](cfg().DisabledClusters...))

	if o.config.JobConfigPath != "" {
		go jobConfigMapMonitor(5*time.Minute, o.config.JobConfigPath)
	}

	metrics.ExposeMetrics("sinker", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	ctrlruntimelog.SetLogger(zap.New(zap.JSONEncoder()))

	infrastructureClusterConfig, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting config for infastructure cluster")
	}

	// The watch apimachinery doesn't support restarts, so just exit the binary if a kubeconfig changes
	// to make the kubelet restart us.
	if err := o.kubernetes.AddKubeconfigChangeCallback(func() {
		logrus.Info("Kubeconfig changed, exiting to trigger a restart")
		interrupts.Terminate()
	}); err != nil {
		logrus.WithError(err).Fatal("Failed to register kubeconfig change callback")
	}

	opts := manager.Options{
		MetricsBindAddress:            "0",
		Namespace:                     cfg().ProwJobNamespace,
		LeaderElection:                true,
		LeaderElectionNamespace:       configAgent.Config().ProwJobNamespace,
		LeaderElectionID:              "prow-sinker-leaderlock",
		LeaderElectionReleaseOnCancel: true,
	}
	mgr, err := manager.New(infrastructureClusterConfig, opts)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating manager")
	}

	// The watch apimachinery doesn't support restarts, so just exit the
	// binary if a build cluster can be connected later.
	callBack := func() {
		logrus.Info("Build cluster that failed to connect initially now worked, exiting to trigger a restart.")
		interrupts.Terminate()
	}

	// We require operating on test pods in build clusters with the following
	// verbs. This is used during startup to check that we have the necessary
	// authorizations on build clusters.
	requiredTestPodVerbs := []string{
		"delete",
		"list",
		"watch",
		"get",
		"patch",
	}

	buildManagers, err := o.kubernetes.BuildClusterManagers(o.dryRun,
		requiredTestPodVerbs,
		// The watch apimachinery doesn't support restarts, so just exit the
		// binary if a build cluster can be connected later .
		callBack,
		func(o *manager.Options) {
			o.Namespace = cfg().PodNamespace
		},
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to construct build cluster managers. Is there a bad entry in the kubeconfig secret?")
	}

	buildClusterClients := map[string]ctrlruntimeclient.Client{}
	for clusterName, buildManager := range buildManagers {
		if err := mgr.Add(buildManager); err != nil {
			logrus.WithError(err).Fatal("Failed to add build cluster manager to main manager")
		}
		buildClusterClients[clusterName] = buildManager.GetClient()
	}

	c := controller{
		ctx:           context.Background(),
		logger:        logrus.NewEntry(logrus.StandardLogger()),
		prowJobClient: mgr.GetClient(),
		podClients:    buildClusterClients,
		config:        cfg,
		runOnce:       o.runOnce,
	}
	if err := mgr.Add(&c); err != nil {
		logrus.WithError(err).Fatal("failed to add controller to manager")
	}
	if err := mgr.Start(interrupts.Context()); err != nil {
		logrus.WithError(err).Fatal("failed to start manager")
	}
	logrus.Info("Manager ended gracefully")
}

type controller struct {
	ctx           context.Context
	logger        *logrus.Entry
	prowJobClient ctrlruntimeclient.Client
	podClients    map[string]ctrlruntimeclient.Client
	config        config.Getter
	runOnce       bool
}

func (c *controller) Start(ctx context.Context) error {
	runChan := make(chan struct{})

	// We want to be able to dynamically adjust to changed config values, hence we cant use a time.Ticker
	go func() {
		for {
			runChan <- struct{}{}
			time.Sleep(c.config().Sinker.ResyncPeriod.Duration)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("stop signal received, quitting")
			return nil
		case <-runChan:
			start := time.Now()
			c.clean()
			c.logger.Infof("Sync time: %v", time.Since(start))
			if c.runOnce {
				return nil
			}
		}
	}
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
		jobConfigMapSize       *prometheus.GaugeVec
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
		jobConfigMapSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "job_configmap_size",
			Help: "Size of ConfigMap storing central job configuration files (gzipped) in bytes.",
		}, []string{
			"name",
		}),
	}
)

func init() {
	prometheus.MustRegister(sinkerMetrics.podsCreated)
	prometheus.MustRegister(sinkerMetrics.timeUsed)
	prometheus.MustRegister(sinkerMetrics.podsRemoved)
	prometheus.MustRegister(sinkerMetrics.podRemovalErrors)
	prometheus.MustRegister(sinkerMetrics.prowJobsCreated)
	prometheus.MustRegister(sinkerMetrics.prowJobsCleaned)
	prometheus.MustRegister(sinkerMetrics.prowJobsCleaningErrors)
	prometheus.MustRegister(sinkerMetrics.jobConfigMapSize)
}

func (m *sinkerReconciliationMetrics) getTimeUsed() time.Duration {
	return m.finishedAt.Sub(m.startAt)
}

func (c *controller) clean() {

	metrics := sinkerReconciliationMetrics{
		startAt:                time.Now(),
		podsRemoved:            map[string]int{},
		podRemovalErrors:       map[string]int{},
		prowJobsCleaned:        map[string]int{},
		prowJobsCleaningErrors: map[string]int{}}

	// Clean up old prow jobs first.
	prowJobs := &prowapi.ProwJobList{}
	if err := c.prowJobClient.List(c.ctx, prowJobs, ctrlruntimeclient.InNamespace(c.config().ProwJobNamespace)); err != nil {
		c.logger.WithError(err).Error("Error listing prow jobs.")
		return
	}
	metrics.prowJobsCreated = len(prowJobs.Items)

	// Only delete pod if its prowjob is marked as finished
	pjMap := map[string]*prowapi.ProwJob{}
	isFinished := sets.New[string]()

	maxProwJobAge := c.config().Sinker.MaxProwJobAge.Duration
	for i, prowJob := range prowJobs.Items {
		pjMap[prowJob.ObjectMeta.Name] = &prowJobs.Items[i]
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
		if err := c.prowJobClient.Delete(c.ctx, &prowJob); err == nil {
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
	for _, p := range c.config().Periodics {
		isActivePeriodic[p.Name] = true
	}

	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestProwJobs(prowJobs.Items, prowapi.PeriodicJob)
	for _, prowJob := range prowJobs.Items {
		if prowJob.Spec.Type != prowapi.PeriodicJob {
			continue
		}

		if !prowJob.Complete() {
			continue
		}
		isFinished.Insert(prowJob.ObjectMeta.Name)
		latestPJ := latestPeriodics[prowJob.Spec.Job]
		if isActivePeriodic[prowJob.Spec.Job] && prowJob.ObjectMeta.Name == latestPJ.ObjectMeta.Name {
			// Ignore deleting this one.
			continue
		}
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if err := c.prowJobClient.Delete(c.ctx, &prowJob); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
			metrics.prowJobsCleaned[reasonProwJobAgedPeriodic]++
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
			metrics.prowJobsCleaningErrors[string(k8serrors.ReasonForError(err))]++
		}
	}

	// Now clean up old pods.
	for cluster, client := range c.podClients {
		log := c.logger.WithField("cluster", cluster)
		var isClusterExcluded bool
		for _, excludeCluster := range c.config().Sinker.ExcludeClusters {
			if excludeCluster == cluster {
				isClusterExcluded = true
				break
			}
		}
		if isClusterExcluded {
			log.Debugf("Cluster %q is excluded, skipping pods deletion.", cluster)
			continue
		}
		var pods corev1api.PodList
		if err := client.List(c.ctx, &pods, ctrlruntimeclient.MatchingLabels{kube.CreatedByProw: "true"}, ctrlruntimeclient.InNamespace(c.config().PodNamespace)); err != nil {
			log.WithError(err).Error("Error listing pods.")
			continue
		}
		log.WithField("pod-count", len(pods.Items)).Debug("Successfully listed pods.")
		metrics.podsCreated += len(pods.Items)
		maxPodAge := c.config().Sinker.MaxPodAge.Duration
		terminatedPodTTL := c.config().Sinker.TerminatedPodTTL.Duration
		for _, pod := range pods.Items {
			reason := ""
			clean := false

			// by default, use the pod name as the key to match the associated prow job
			// this is to support legacy plank in case the kube.ProwJobIDLabel label is not set
			podJobName := pod.ObjectMeta.Name
			// if the pod has the kube.ProwJobIDLabel label, use this instead of the pod name
			if value, ok := pod.ObjectMeta.Labels[kube.ProwJobIDLabel]; ok {
				podJobName = value
			}
			log = log.WithField("pj", podJobName)
			terminationTime := time.Time{}
			if pj, ok := pjMap[podJobName]; ok && pj.Complete() {
				terminationTime = pj.Status.CompletionTime.Time
			}

			if podNeedsKubernetesFinalizerCleanup(log, pjMap[podJobName], &pod) {
				if err := c.cleanupKubernetesFinalizer(&pod, client); err != nil {
					log.WithError(err).Error("Failed to remove kubernetesreporter finalizer")
				}
			}

			switch {
			case !pod.Status.StartTime.IsZero() && time.Since(pod.Status.StartTime.Time) > maxPodAge:
				clean = true
				reason = reasonPodAged
			case !terminationTime.IsZero() && time.Since(terminationTime) > terminatedPodTTL:
				clean = true
				reason = reasonPodTTLed
			}

			if !isFinished.Has(podJobName) {
				// prowjob exists and is not marked as completed yet
				// deleting the pod now will result in plank creating a brand new pod
				clean = false
			}

			if c.isPodOrphaned(log, &pod, podJobName) {
				// prowjob has gone, we want to clean orphan pods regardless of the state
				reason = reasonPodOrphaned
				clean = true
			}

			if !clean {
				continue
			}

			c.deletePod(log, &pod, reason, client, &metrics)
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

func (c *controller) cleanupKubernetesFinalizer(pod *corev1api.Pod, client ctrlruntimeclient.Client) error {

	oldPod := pod.DeepCopy()
	pod.Finalizers = sets.List(sets.New[string](pod.Finalizers...).Delete(kubernetesreporterapi.FinalizerName))

	if err := client.Patch(c.ctx, pod, ctrlruntimeclient.MergeFrom(oldPod)); err != nil {
		return fmt.Errorf("failed to patch pod: %w", err)
	}

	return nil
}

func (c *controller) deletePod(log *logrus.Entry, pod *corev1api.Pod, reason string, client ctrlruntimeclient.Client, m *sinkerReconciliationMetrics) {
	name := pod.Name
	// Delete old finished or orphan pods. Don't quit if we fail to delete one.
	if err := client.Delete(c.ctx, pod); err == nil {
		log.WithFields(logrus.Fields{"pod": name, "reason": reason}).Info("Deleted old completed pod.")
		m.podsRemoved[reason]++
	} else {
		m.podRemovalErrors[string(k8serrors.ReasonForError(err))]++
		if k8serrors.IsNotFound(err) {
			log.WithField("pod", name).WithError(err).Info("Could not delete missing pod.")
		} else {
			log.WithField("pod", name).WithError(err).Error("Error deleting pod.")
		}
	}
}

func (c *controller) isPodOrphaned(log *logrus.Entry, pod *corev1api.Pod, prowJobName string) bool {
	// ProwJobs are cached and the cache may lag a bit behind, so never considers
	// pods that are less than 30 seconds old as orphaned
	if !pod.CreationTimestamp.Before(&metav1.Time{Time: time.Now().Add(-30 * time.Second)}) {
		return false
	}

	// We do a list in the very beginning of our processing. By the time we reach this check, that
	// list might be outdated, so do another GET here before declaring the pod orphaned
	pjName := types.NamespacedName{Namespace: c.config().ProwJobNamespace, Name: prowJobName}
	if err := c.prowJobClient.Get(c.ctx, pjName, &prowapi.ProwJob{}); err != nil {
		if k8serrors.IsNotFound(err) {
			return true
		}
		logrus.WithError(err).Error("Failed to get prowjob")
	}

	return false
}

func podNeedsKubernetesFinalizerCleanup(log *logrus.Entry, pj *prowapi.ProwJob, pod *corev1api.Pod) bool {
	// Can happen if someone deletes the prowjob before it finishes
	if pj == nil {
		return true
	}
	// This is always a bug
	if pj.Complete() && pj.Status.PrevReportStates[kubernetesreporterapi.ReporterName] == pj.Status.State && sets.New[string](pod.Finalizers...).Has(kubernetesreporterapi.FinalizerName) {
		log.WithField("pj", pj.Name).Errorf("BUG: Pod for prowjob still had the %s finalizer after completing and being successfully reported by the %s reporter", kubernetesreporterapi.FinalizerName, kubernetesreporterapi.ReporterName)

		return true
	}

	return false
}

// jobConfigMapMonitor reports metrics for the size of the ConfigMap(s) found
// under the the directory specified with --job-config-path (example:
// "--job-config-path=/etc/job-config"). There are two possibilities --- either
// the job ConfigMap is mounted directly at that path, or the ConfigMap was
// partitioned (see https://github.com/kubernetes/test-infra/pull/28835) and
// there are multiple subdirs underneath this one.
func jobConfigMapMonitor(interval time.Duration, jobConfigPath string) {
	logger := logrus.WithField("sync-loop", "job-configmap-monitor")
	ticker := time.NewTicker(interval)

	for ; true; <-ticker.C {
		dirs, err := getConfigMapDirs(jobConfigPath)
		if err != nil {
			logger.WithField("dir", jobConfigPath).Error("could not resolve ConfigMap dirs")
			continue
		}
		for _, dir := range dirs {
			bytes, err := getConfigMapSize(dir)
			if err != nil {
				logger.WithField("dir", dir).WithError(err).Error("Failed to get configmap metrics")
				continue
			}
			sinkerMetrics.jobConfigMapSize.WithLabelValues(dir).Set(float64(bytes))
		}
	}
}

// getDataDir gets the "..data" symlink which points to a timestamped directory.
// See the comment for getConfigMapSize() for details.
func getDataDir(toplevel string) string {
	return path.Join(toplevel, "..data")
}

func getConfigMapDirs(toplevel string) ([]string, error) {
	dataDir := getDataDir(toplevel)
	dirs := []string{}

	// If the data dir (symlink) does not exist directly, then assume that this
	// path is a partition holding multiple ConfigMap-mounted dirs. We use
	// os.Stat(), which means that both the "..data" symlink and its target
	// folder must exist. Of course, nothing stops the folder from having
	// "..data" as a folder or regular file, which would count as false
	// positives, but we ignore these cases because exhaustive checking here is
	// not our concern. We just want metrics.
	if _, err := os.Stat(dataDir); errors.Is(err, os.ErrNotExist) {
		files, err := os.ReadDir(toplevel)
		if err != nil {
			return nil, err
		}

		for _, file := range files {
			if !file.IsDir() {
				continue
			}
			dirs = append(dirs, filepath.Join(toplevel, file.Name()))
		}
	} else {
		dirs = append(dirs, toplevel)
	}

	return dirs, nil
}

// getConfigMapSize expects a path to the filesystem where a Kubernetes
// ConfigMap has been mounted. It iterates over every key (file) found in that
// directory, adding up the sizes of each of the files by calling
// "syscall.Stat".
//
// When ConfigMaps are mounted to disk, all of its keys will become files
// and the value (data) for each key will be the contents of the respective
// files. Another special symlink, `..data`, will also be at the same level
// as the keys and this symlink will point to yet another folder at the same
// level like `..2024_01_11_22_52_09.1709975282`. This timestamped folder is
// where the actual files will be located. So the layout looks like:
//
// folder-named-after-configmap-name
// folder-named-after-configmap-name/..2024_01_11_22_52_09.1709975282
// folder-named-after-configmap-name/..data (symlinked to ..2024_01_11... above)
// folder-named-after-configmap-name/key1 (symlinked to ..data/key1)
// folder-named-after-configmap-name/key2 (symlinked to ..data/key2)
//
// The above layout with the timestamped folder and the "..data" symlink is a
// Kubernetes construct, and is applicable to every ConfigMap mounted to disk by
// Kubernetes.
//
// For our purposes the exact details of this doesn't matter too much ---
// our call to syscall.Stat() will still work for key1 and key2 above even
// though they are symlinks. What we do care about though is the existence
// of such `..data` and `..<timestamp>` files. We have to exclude these
// files from our totals because otherwise we'd be double counting.
func getConfigMapSize(configmapDir string) (int64, error) {
	var total int64

	// Look into the "..data" symlinked folder, which should contain the actual
	// files where each file is a key in the ConfigMap.
	dataDir := getDataDir(configmapDir)
	if _, err := os.Stat(dataDir); errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("%q is not a ConfigMap-mounted dir", configmapDir)
	}

	logger := logrus.NewEntry(logrus.StandardLogger())

	var walkDirFunc = func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Don't process directories (that is, only process files). We don't
		// expect any directories to exist at this level, but it doesn't hurt to
		// skip any we encounter.
		if d.IsDir() {
			return nil
		}
		// Skip any symbolic links.
		if d.Type() == fs.ModeSymlink {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		logger.Infof("file %q is %v bytes", path, info.Size())
		total += info.Size()
		return nil
	}

	if err := filepath.WalkDir(configmapDir, walkDirFunc); err != nil {
		return 0, nil
	}

	return total, nil
}
