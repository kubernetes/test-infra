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

package jira

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/test-infra/prow/simplifypath"
)

func init() {
	prometheus.MustRegister(requestResults)
}

var requestResults = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "jira_request_duration_seconds",
	Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20},
}, []string{methodField, pathField, statusField})

func pathSimplifier() simplifypath.Simplifier {
	return simplifypath.NewSimplifier(simplifypath.L("", // shadow element mimicing the root
		simplifypath.L("rest",
			simplifypath.L("api",
				simplifypath.L("2",
					simplifypath.L("project"),
					simplifypath.L("issue",
						simplifypath.V("issueID",
							simplifypath.L("remotelink"),
						),
					),
				),
			),
		),
	))
}

const (
	methodField = "method"
	pathField   = "path"
	statusField = "status"
)

type metricsTransport struct {
	upstream       http.RoundTripper
	pathSimplifier func(string) string
	recorder       *prometheus.HistogramVec
}

func (m *metricsTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	result, err := m.upstream.RoundTrip(r)
	status := "error"
	if err == nil && result != nil {
		status = strconv.Itoa(result.StatusCode)
	}
	var path string
	if r.URL != nil {
		path = r.URL.Path
	}
	m.recorder.WithLabelValues(r.Method, m.pathSimplifier(path), status).Observe(float64(time.Since(start).Milliseconds()) / 1000)
	return result, err
}
