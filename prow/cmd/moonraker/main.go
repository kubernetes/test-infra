/*
Copyright 2022 The Kubernetes Authors.

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

// "Moonraker" is a caching service to cache inrepoconfig (ProwYAML) objects. It
// handles cloning the Git repo holding the inrepoconfig, parsing the ProwYAML
// out of it, as well as caching the result in an in-memory LRU cache. Other
// Prow components in the same service cluster can go through Moonraker to save
// the trouble of trying to perform inrepoconfig lookups themselves.

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/diskutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/moonraker"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pjutil/pprof"
)

var (
	diskFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_free",
		Help: "Free gb on moonraker disk.",
	})
	diskUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_used",
		Help: "Used gb on moonraker disk.",
	})
	diskTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_total",
		Help: "Total gb on moonraker disk.",
	})
	diskInodeFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_inode_free",
		Help: "Free inodes on moonraker disk.",
	})
	diskInodeUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_inode_used",
		Help: "Used inodes on moonraker disk.",
	})
	diskInodeTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "moonraker_disk_inode_total",
		Help: "Total inodes on moonraker disk.",
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
	github         prowflagutil.GitHubOptions
	port           int
	cookiefilePath string

	config configflagutil.ConfigOptions

	dryRun                 bool
	gracePeriod            time.Duration
	instrumentationOptions prowflagutil.InstrumentationOptions
	pushGatewayInterval    time.Duration
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 8080, "HTTP port.")
	// Kubernetes uses a 30-second default grace period for pods to
	// terminate before sending a SIGKILL to the process in the pod. Our own
	// grace period must be smaller than this.
	fs.DurationVar(&o.gracePeriod, "grace-period", 25*time.Second, "On shutdown, try to handle remaining events for the specified duration. Cannot be larger than 30s.")
	fs.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for github or anonymous")
	fs.DurationVar(&o.pushGatewayInterval, "push-gateway-interval", time.Minute, "Interval at which prometheus metrics for disk space are pushed.")
	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions, &o.config} {
		group.AddFlags(fs)
	}

	fs.Parse(args)

	return o
}

func (o *options) validate() error {
	var errs []error
	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions, &o.config} {
		if err := group.Validate(o.dryRun); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	pprof.Instrument(o.instrumentationOptions)

	// Start serving liveness endpoint /healthz.
	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	metrics.ExposeMetrics("moonraker", configAgent.Config().PushGateway, o.instrumentationOptions.MetricsPort)

	persist := false
	if o.config.InRepoConfigCacheDirBase != "" {
		persist = true
	}

	gitClient, err := o.github.GitClientFactory(o.cookiefilePath, &o.config.InRepoConfigCacheDirBase, o.dryRun, persist)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}

	if o.config.InRepoConfigCacheDirBase != "" {
		go diskMonitor(o.pushGatewayInterval, o.config.InRepoConfigCacheDirBase)
	}

	cacheGetter, err := config.NewInRepoConfigCache(o.config.InRepoConfigCacheSize, configAgent, gitClient)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating InRepoConfigCacheGetter.")
	}

	mr := moonraker.Moonraker{
		ConfigAgent:       configAgent,
		InRepoConfigCache: cacheGetter,
	}

	// If the main config changes (an update to the ConfigMap holding the main
	// config), we have to reload it because the "in_repo_config" setting which
	// allowlists repositories may have changed (a repository may have been
	// enabled or disabled from inrepoconfig). We have to check this setting
	// before we do the clone or fetch.
	logrus.Info("Setting up ConfigWatcher")
	interrupts.Run(func(ctx context.Context) {
		if err := mr.RunConfigWatcher(ctx); err != nil {
			logrus.WithError(err).Fatal("Failed to run ConfigWatcher")
		}
	})

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/%s", moonraker.PathPing), mr.ServePing)
	mux.HandleFunc(fmt.Sprintf("/%s", moonraker.PathGetInrepoconfig), mr.ServeGetInrepoconfig)
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(o.port),
		Handler: mux,
	}
	logrus.Infof("Listening on port %d...", o.port)
	interrupts.ListenAndServe(server, o.gracePeriod)
	health.ServeReady(func() bool {
		return true
	})
	interrupts.WaitForGracefulShutdown()
}

// diskMonitor was copied from ghproxy.
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
