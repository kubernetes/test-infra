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
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/ghproxy/apptokenequalizer"
	"k8s.io/test-infra/ghproxy/ghcache"
	"k8s.io/test-infra/greenhouse/diskutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

var (
	diskFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_free",
		Help: "Free gb on github-cache disk",
	})
	diskUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_used",
		Help: "Used gb on github-cache disk",
	})
	diskTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_total",
		Help: "Total gb on github-cache disk",
	})
)

func init() {
	prometheus.MustRegister(diskFree)
	prometheus.MustRegister(diskUsed)
	prometheus.MustRegister(diskTotal)
}

// GitHub reverse proxy HTTP cache RoundTripper stack:
//  v -   <Client(s)>
//  v ^ reverse proxy
//  v ^ ghcache: downstreamTransport (coalescing, instrumentation)
//  v ^ ghcache: httpcache layer
//  v ^ ghcache: upstreamTransport (cache-control, instrumentation)
//  v ^ apptokenequalizer: Make sure all clients get the same app installation token so they can share a cache
//  v ^ http.DefaultTransport
//  > ^   <Upstream>

type options struct {
	dir                                    string
	sizeGB                                 int
	diskCacheDisableAuthHeaderPartitioning bool

	redisAddress string

	port           int
	upstream       string
	upstreamParsed *url.URL

	maxConcurrency int

	// pushGateway fields are used to configure pushing prometheus metrics.
	pushGateway         string
	pushGatewayInterval time.Duration

	logLevel string

	serveMetrics bool

	instrumentationOptions flagutil.InstrumentationOptions
}

func (o *options) validate() error {
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level specified: %v", err)
	}
	logrus.SetLevel(level)

	if (o.dir == "") != (o.sizeGB == 0) {
		return errors.New("--cache-dir and --cache-sizeGB must be specified together to enable the disk cache (otherwise a memory cache is used)")
	}
	upstreamURL, err := url.Parse(o.upstream)
	if err != nil {
		return fmt.Errorf("failed to parse upstream URL: %v", err)
	}
	o.upstreamParsed = upstreamURL
	return nil
}

func flagOptions() *options {
	o := &options{}
	flag.StringVar(&o.dir, "cache-dir", "", "Directory to cache to if using a disk cache.")
	flag.IntVar(&o.sizeGB, "cache-sizeGB", 0, "Cache size in GB per unique token if using a disk cache.")
	flag.BoolVar(&o.diskCacheDisableAuthHeaderPartitioning, "legacy-disable-disk-cache-partitions-by-auth-header", true, "Whether to disable partitioning a disk cache by auth header. Disabling this will start a new cache at $cache_dir/$sha256sum_of_authorization_header for each unique authorization header. Bigger setups are advise to manually warm this up from an existing cache. This option will be removed and set to `false` in the future")
	flag.StringVar(&o.redisAddress, "redis-address", "", "Redis address if using a redis cache e.g. localhost:6379.")
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.StringVar(&o.upstream, "upstream", "https://api.github.com", "Scheme, host, and base path of reverse proxy upstream.")
	flag.IntVar(&o.maxConcurrency, "concurrency", 25, "Maximum number of concurrent in-flight requests to GitHub.")
	flag.StringVar(&o.pushGateway, "push-gateway", "", "If specified, push prometheus metrics to this endpoint.")
	flag.DurationVar(&o.pushGatewayInterval, "push-gateway-interval", time.Minute, "Interval at which prometheus metrics are pushed.")
	flag.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	flag.BoolVar(&o.serveMetrics, "serve-metrics", false, "If true, it serves prometheus metrics")
	o.instrumentationOptions.AddFlags(flag.CommandLine)
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := flagOptions()
	flag.Parse()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}

	if o.diskCacheDisableAuthHeaderPartitioning {
		logrus.Warningf("The deprecated `--legacy-disable-disk-cache-partitions-by-auth-header` flags value is `true`. If you are a bigger Prow setup, you should copy your existing cache directory to the directory mentioned in the `%s` messages to warm up the partitioned-by-auth-header cache, then set the flag to false. If you are a smaller Prow setup or just started using ghproxy you can just unconditionally set it to `false`.", ghcache.LogMessageWithDiskPartitionFields)
	}

	var cache http.RoundTripper
	if o.redisAddress != "" {
		cache = ghcache.NewRedisCache(apptokenequalizer.New(http.DefaultTransport), o.redisAddress, o.maxConcurrency)
	} else if o.dir == "" {
		cache = ghcache.NewMemCache(apptokenequalizer.New(http.DefaultTransport), o.maxConcurrency)
	} else {
		cache = ghcache.NewDiskCache(apptokenequalizer.New(http.DefaultTransport), o.dir, o.sizeGB, o.maxConcurrency, o.diskCacheDisableAuthHeaderPartitioning)
		go diskMonitor(o.pushGatewayInterval, o.dir)
	}

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)
	defer interrupts.WaitForGracefulShutdown()
	metrics.ExposeMetrics("ghproxy", config.PushGateway{
		Endpoint: o.pushGateway,
		Interval: &metav1.Duration{
			Duration: o.pushGatewayInterval,
		},
		ServeMetrics: o.serveMetrics,
	}, o.instrumentationOptions.MetricsPort)

	proxy := newReverseProxy(o.upstreamParsed, cache, 30*time.Second)
	server := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: proxy}

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	interrupts.ListenAndServe(server, 30*time.Second)
}

func newReverseProxy(upstreamURL *url.URL, transport http.RoundTripper, timeout time.Duration) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	// Wrap the director to change the upstream request 'Host' header to the
	// target host.
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = req.URL.Host
	}
	proxy.Transport = transport

	return http.TimeoutHandler(proxy, timeout, fmt.Sprintf("ghproxy timed out after %v", timeout))
}

// helper to update disk metrics (copied from greenhouse)
func diskMonitor(interval time.Duration, diskRoot string) {
	logger := logrus.WithField("sync-loop", "disk-monitor")
	ticker := time.NewTicker(interval)
	for ; true; <-ticker.C {
		logger.Info("tick")
		_, bytesFree, bytesUsed, err := diskutil.GetDiskUsage(diskRoot)
		if err != nil {
			logger.WithError(err).Error("Failed to get disk metrics")
		} else {
			diskFree.Set(float64(bytesFree) / 1e9)
			diskUsed.Set(float64(bytesUsed) / 1e9)
			diskTotal.Set(float64(bytesFree+bytesUsed) / 1e9)
		}
	}
}
