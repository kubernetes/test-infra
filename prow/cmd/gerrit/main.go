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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/greenhouse/diskutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/gerrit/adapter"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
)

var (
	diskFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gerrit_disk_free",
		Help: "Free gb on gerrit-cache disk.",
	})
	diskUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gerrit_disk_used",
		Help: "Used gb on gerrit-cache disk.",
	})
	diskTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_total",
		Help: "Total gb on gerrit-cache disk.",
	})
	diskInodeFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_inode_free",
		Help: "Free inodes on gerrit-cache disk.",
	})
	diskInodeUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gerrit_disk_inode_used",
		Help: "Used inodes on gerrit-cache disk.",
	})
	diskInodeTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gerrit_disk_inode_total",
		Help: "Total inodes on gerrit-cache disk.",
	})
)

func init() {
	prometheus.MustRegister(diskFree)
	prometheus.MustRegister(diskUsed)
	prometheus.MustRegister(diskTotal)
	prometheus.MustRegister(diskInodeFree)
	prometheus.MustRegister(diskInodeUsed)
	prometheus.MustRegister(diskInodeTotal)
}

type options struct {
	cookiefilePath    string
	tokenPathOverride string
	config            configflagutil.ConfigOptions
	// lastSyncFallback is the path to sync the latest timestamp
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	lastSyncFallback       string
	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
	storage                prowflagutil.StorageClientOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	changeWorkerPoolSize   int
	pushGatewayInterval    time.Duration
}

func (o *options) validate() error {
	if o.cookiefilePath != "" && o.tokenPathOverride != "" {
		return fmt.Errorf("only one of --cookiefile=%q --token-path=%q allowed, not both", o.cookiefilePath, o.tokenPathOverride)
	}
	if o.cookiefilePath == "" && o.tokenPathOverride == "" {
		logrus.Info("--cookiefile is not set, using anonymous authentication")
	}

	if err := o.config.Validate(o.dryRun); err != nil {
		return err
	}

	if o.lastSyncFallback == "" {
		return errors.New("--last-sync-fallback must be set")
	}

	if strings.HasPrefix(o.lastSyncFallback, "gs://") && !o.storage.HasGCSCredentials() {
		logrus.WithField("last-sync-fallback", o.lastSyncFallback).Info("--gcs-credentials-file unset, will try and access with a default service account")
	}
	if strings.HasPrefix(o.lastSyncFallback, "s3://") && !o.storage.HasS3Credentials() {
		logrus.WithField("last-sync-fallback", o.lastSyncFallback).Info("--s3-credentials-file unset, will try and access with auto-discovered credentials")
	}
	if o.changeWorkerPoolSize < 1 {
		return errors.New("change-worker-pool-size must be at least 1")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for anonymous")
	fs.StringVar(&o.lastSyncFallback, "last-sync-fallback", "", "The /local/path, gs://path/to/object or s3://path/to/object to sync the latest timestamp")
	fs.BoolVar(&o.dryRun, "dry-run", false, "Run in dry-run mode, performing no modifying actions.")
	fs.StringVar(&o.tokenPathOverride, "token-path", "", "Force the use of the token in this path, use with gcloud auth print-access-token")
	fs.IntVar(&o.changeWorkerPoolSize, "change-worker-pool-size", 1, "Number of workers processing changes for each instance.")
	fs.DurationVar(&o.pushGatewayInterval, "push-gateway-interval", time.Minute, "Interval at which prometheus metrics for disk space are pushed.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.storage, &o.instrumentationOptions, &o.config} {
		group.AddFlags(fs)
	}
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	runtime.SetBlockProfileRate(100_000_000) // 0.1 second sample rate https://github.com/DataDog/go-profiler-notes/blob/main/guide/README.md#block-profiler-limitations
	pprof.Instrument(o.instrumentationOptions)

	ca, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := ca.Config

	// Expose Prometheus metrics
	metrics.ExposeMetrics("gerrit", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	prowJobClient, err := o.kubernetes.ProwJobClient(cfg().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	ctx := context.Background() // TODO(fejta): use something better
	op, err := o.storage.StorageClient(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating opener")
	}

	persist := false
	if o.config.InRepoConfigCacheDirBase != "" {
		persist = true
	}

	gitClient, err := (&prowflagutil.GitHubOptions{}).GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun, persist)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating git client.")
	}

	if o.config.InRepoConfigCacheDirBase != "" {
		go diskMonitor(o.pushGatewayInterval, o.config.InRepoConfigCacheDirBase)
	}

	cacheGetter, err := config.NewInRepoConfigCache(o.config.InRepoConfigCacheSize, ca, gitClient)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}
	c := adapter.NewController(ctx, prowJobClient, op, ca, o.cookiefilePath, o.tokenPathOverride, o.lastSyncFallback, o.changeWorkerPoolSize, cacheGetter)

	logrus.Infof("Starting gerrit fetcher")

	defer interrupts.WaitForGracefulShutdown()
	interrupts.Tick(func() {
		c.Sync()
	}, func() time.Duration {
		return cfg().Gerrit.TickInterval.Duration
	})
}

// helper to update disk metrics (copied from ghproxy)
func diskMonitor(interval time.Duration, diskRoot string) {
	logger := logrus.WithField("sync-loop", "disk-monitor")
	ticker := time.NewTicker(interval)
	for ; true; <-ticker.C {
		logger.Info("tick")
		_, bytesFree, bytesUsed, _, inodesFree, inodesUsed, err := diskutil.GetDiskUsage(diskRoot)
		if err != nil {
			logger.WithError(err).Error("Failed to get disk metrics")
		} else {
			diskFree.Set(float64(bytesFree) / 1e9)
			diskUsed.Set(float64(bytesUsed) / 1e9)
			diskTotal.Set(float64(bytesFree+bytesUsed) / 1e9)
			diskInodeFree.Set(float64(inodesFree))
			diskInodeUsed.Set(float64(inodesUsed))
			diskInodeTotal.Set(float64(inodesFree + inodesUsed))
		}
	}
}
