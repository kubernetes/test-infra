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
	"k8s.io/test-infra/prow/pjutil/pprof"

	"k8s.io/test-infra/ghproxy/apptokenequalizer"
	"k8s.io/test-infra/ghproxy/ghcache"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/diskutil"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

var (
	diskFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_free",
		Help: "Free gb on github-cache disk.",
	})
	diskUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_used",
		Help: "Used gb on github-cache disk.",
	})
	diskTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_total",
		Help: "Total gb on github-cache disk.",
	})
	diskInodeFree = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_inode_free",
		Help: "Free inodes on github-cache disk.",
	})
	diskInodeUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_inode_used",
		Help: "Used inodes on github-cache disk.",
	})
	diskInodeTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ghcache_disk_inode_total",
		Help: "Total inodes on github-cache disk.",
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

	maxConcurrency                  int
	requestThrottlingTime           uint
	requestThrottlingTimeV4         uint
	requestThrottlingTimeForGET     uint
	requestThrottlingMaxDelayTime   uint
	requestThrottlingMaxDelayTimeV4 uint

	// pushGateway fields are used to configure pushing prometheus metrics.
	pushGateway         string
	pushGatewayInterval time.Duration

	logLevel string

	serveMetrics bool

	instrumentationOptions flagutil.InstrumentationOptions

	timeout uint
}

func (o *options) validate() error {
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level specified: %w", err)
	}
	logrus.SetLevel(level)

	if (o.dir == "") != (o.sizeGB == 0) {
		return errors.New("--cache-dir and --cache-sizeGB must be specified together to enable the disk cache (otherwise a memory cache is used)")
	}
	upstreamURL, err := url.Parse(o.upstream)
	if err != nil {
		return fmt.Errorf("failed to parse upstream URL: %w", err)
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
	flag.UintVar(&o.requestThrottlingTime, "throttling-time-ms", 0, "Additional throttling mechanism which imposes time spacing between outgoing requests. Counted per organization. Has to be set together with --get-throttling-time-ms.")
	flag.UintVar(&o.requestThrottlingTimeV4, "throttling-time-v4-ms", 0, "Additional throttling mechanism which imposes time spacing between outgoing requests. Counted per organization. Overrides --throttling-time-ms setting for API v4.")
	flag.UintVar(&o.requestThrottlingTimeForGET, "get-throttling-time-ms", 0, "Additional throttling mechanism which imposes time spacing between outgoing GET requests. Counted per organization. Has to be set together with --throttling-time-ms.")
	flag.UintVar(&o.requestThrottlingMaxDelayTime, "throttling-max-delay-duration-seconds", 30, "Maximum delay for throttling in seconds. Requests will never be throttled for longer than this, used to avoid building a request backlog when the GitHub api has performance issues. Default is 30 seconds.")
	flag.UintVar(&o.requestThrottlingMaxDelayTimeV4, "throttling-max-delay-duration-v4-seconds", 30, "Maximum delay for throttling in seconds for APIv4. Requests will never be throttled for longer than this, used to avoid building a request backlog when the GitHub api has performance issues. Default is 30 seconds.")
	flag.StringVar(&o.pushGateway, "push-gateway", "", "If specified, push prometheus metrics to this endpoint.")
	flag.DurationVar(&o.pushGatewayInterval, "push-gateway-interval", time.Minute, "Interval at which prometheus metrics are pushed.")
	flag.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	flag.BoolVar(&o.serveMetrics, "serve-metrics", false, "If true, it serves prometheus metrics")
	flag.UintVar(&o.timeout, "request-timeout", 30, "Request timeout which applies also to paged requests. Default is 30 seconds.")
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

	if (o.requestThrottlingTime > 0 && o.requestThrottlingTimeForGET == 0) ||
		(o.requestThrottlingTime == 0 && o.requestThrottlingTimeForGET > 0) ||
		((o.requestThrottlingTime == 0 || o.requestThrottlingTimeForGET == 0) && o.requestThrottlingTimeV4 > 0) {
		logrus.Warningln("Flags `--throttling-time-ms` and `--get-throttling-time-ms` have to be set to non-zero value, otherwise throttling feature will be disabled.")
	}

	pprof.Instrument(o.instrumentationOptions)
	defer interrupts.WaitForGracefulShutdown()
	metrics.ExposeMetrics("ghproxy", config.PushGateway{
		Endpoint: o.pushGateway,
		Interval: &metav1.Duration{
			Duration: o.pushGatewayInterval,
		},
		ServeMetrics: o.serveMetrics,
	}, o.instrumentationOptions.MetricsPort)

	proxy := proxy(o, http.DefaultTransport, time.Hour)
	server := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: proxy}

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	interrupts.ListenAndServe(server, time.Duration(o.timeout)*time.Second)
}

func proxy(o *options, upstreamTransport http.RoundTripper, diskCachePruneInterval time.Duration) http.Handler {
	var cache http.RoundTripper
	throttlingTimes := ghcache.NewRequestThrottlingTimes(o.requestThrottlingTime, o.requestThrottlingTimeV4, o.requestThrottlingTimeForGET, o.requestThrottlingMaxDelayTime, o.requestThrottlingMaxDelayTimeV4)
	if o.redisAddress != "" {
		cache = ghcache.NewRedisCache(apptokenequalizer.New(upstreamTransport), o.redisAddress, o.maxConcurrency, throttlingTimes)
	} else if o.dir == "" {
		cache = ghcache.NewMemCache(apptokenequalizer.New(upstreamTransport), o.maxConcurrency, throttlingTimes)
	} else {
		cache = ghcache.NewDiskCache(apptokenequalizer.New(upstreamTransport), o.dir, o.sizeGB, o.maxConcurrency, o.diskCacheDisableAuthHeaderPartitioning, diskCachePruneInterval, throttlingTimes)
		go diskMonitor(o.pushGatewayInterval, o.dir)
	}

	return newReverseProxy(o.upstreamParsed, cache, time.Duration(o.timeout)*time.Second)
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
