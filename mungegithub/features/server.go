/*
Copyright 2017 The Kubernetes Authors.

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

package features

import (
	"net/http"
	"os"

	"github.com/NYTimes/gziphandler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/options"
	"k8s.io/test-infra/mungegithub/sharedmux"
)

const (
	ServerFeatureName = "server"
)

// ServerFeature runs a server and allows mungers to register handlers for paths, or
// prometheus metrics.
type ServerFeature struct {
	*sharedmux.ConcurrentMux
	Enabled bool

	Address string
	WWWRoot string

	prometheus struct {
		loops      prometheus.Counter
		tokenUsage prometheus.Gauge
	}

	getTokenUsage func() int
}

func init() {
	s := &ServerFeature{}
	RegisterFeature(s)
}

// Name is just going to return the name mungers use to request this feature
func (s *ServerFeature) Name() string {
	return ServerFeatureName
}

// Initialize will initialize the feature.
func (s *ServerFeature) Initialize(config *github.Config) error {
	config.ServeDebugStats("/stats")
	s.ConcurrentMux = sharedmux.NewConcurrentMux(http.DefaultServeMux)
	if len(s.Address) == 0 {
		return nil
	}
	if len(s.WWWRoot) > 0 {
		wwwStat, err := os.Stat(s.WWWRoot)
		if !os.IsNotExist(err) && wwwStat.IsDir() {
			s.ConcurrentMux.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir(s.WWWRoot))))
		}
	}
	// config indicates that ServerFeature should be enabled.
	s.Enabled = true

	s.prometheus.loops = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "mungegithub_loops",
		Help:        "Number of loops performed by the munger",
		ConstLabels: map[string]string{"org": config.Org, "repo": config.Project, "app": mungeopts.App},
	})
	s.prometheus.tokenUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "github_token_usage",
		Help:        "Number of github tokens used by the munger",
		ConstLabels: map[string]string{"org": config.Org, "repo": config.Project, "app": mungeopts.App},
	})
	prometheus.MustRegister(s.prometheus.loops)
	prometheus.MustRegister(s.prometheus.tokenUsage)
	s.getTokenUsage = config.GetTokenUsage
	s.ConcurrentMux.Handle("/prometheus", promhttp.Handler())

	go http.ListenAndServe(s.Address, s.ConcurrentMux)
	return nil
}

// EachLoop is called at the start of every munge loop
func (s *ServerFeature) EachLoop() error {
	s.prometheus.loops.Inc()
	s.prometheus.tokenUsage.Set(float64(s.getTokenUsage()))
	return nil
}

// RegisterOptions registers options for this feature; returns any that require a restart when changed.
func (s *ServerFeature) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&s.Address, "address", ":8080", "The address to listen on for HTTP Status")
	opts.RegisterString(&s.WWWRoot, "www", "www", "Path to static web files to serve from the webserver")
	return sets.NewString("address", "www")
}
