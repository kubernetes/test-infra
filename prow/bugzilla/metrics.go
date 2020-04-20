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

package bugzilla

import "github.com/prometheus/client_golang/prometheus"

// requestDurations provides the 'bugzilla_request_duration' histogram that keeps track
// of the duration of Bugzilla requests by API path.
var requestDurations = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "bugzilla_request_duration",
		Help:    "Bugzilla request duration by API path.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	},
	[]string{methodField, "status"},
)

func init() {
	prometheus.MustRegister(requestDurations)
}
