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

package metrics

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/test-infra/prow/simplifypath"
)

func HttpRequestDuration(prefix string, min, max float64) *prometheus.HistogramVec {
	return histogram(
		prefix+"_http_request_duration_seconds",
		"http request duration in seconds",
		powersOfTwoBetween(min, max),
	)
}

func HttpResponseSize(prefix string, min, max int) *prometheus.HistogramVec {
	return histogram(
		prefix+"_http_response_size_bytes",
		"http response size in bytes",
		powersOfTwoBetween(float64(min), float64(max)),
	)
}

// powersOfTwoBetween returns a set containing min, max and all the integer powers
// of two between them, including negative integers if either the min or max is <1
func powersOfTwoBetween(min, max float64) []float64 {
	var powers []float64
	floor, ceiling := math.Ceil(math.Log2(min)), math.Floor(math.Log2(max))
	if math.Pow(2, floor) != min {
		powers = append(powers, min)
	}
	for i := floor; i <= ceiling; i++ {
		powers = append(powers, math.Pow(2, i))
	}
	if math.Pow(2, ceiling) != max {
		powers = append(powers, max)
	}
	return powers
}

func histogram(name, help string, buckets []float64) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: buckets,
		},
		[]string{"path", "method", "status", "user_agent"},
	)
}

type traceResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (trw *traceResponseWriter) WriteHeader(code int) {
	trw.statusCode = code
	trw.ResponseWriter.WriteHeader(code)
}

func (trw *traceResponseWriter) Write(data []byte) (int, error) {
	size, err := trw.ResponseWriter.Write(data)
	trw.size += size
	return size, err
}

func TraceHandler(simplifier simplifypath.Simplifier, httpRequestDuration, httpResponseSize *prometheus.HistogramVec) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := time.Now()
			// Initialize the status to 200 in case WriteHeader is not called
			trw := &traceResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			h.ServeHTTP(trw, r)
			latency := time.Since(t)
			labels := prometheus.Labels{"path": simplifier.Simplify(r.URL.Path), "method": r.Method, "status": strconv.Itoa(trw.statusCode), "user_agent": r.Header.Get("User-Agent")}
			httpRequestDuration.With(labels).Observe(latency.Seconds())
			httpResponseSize.With(labels).Observe(float64(trw.size))
		})
	}
}
