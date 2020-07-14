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
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/test-infra/prow/simplifypath"
	"k8s.io/utils/diff"
)

func TestPowersOfTwoBetween(t *testing.T) {
	var testCases = []struct {
		name     string
		min, max float64
		powers   []float64
	}{
		{
			name:   "bounds are powers",
			min:    2,
			max:    32,
			powers: []float64{2, 4, 8, 16, 32},
		},
		{
			name:   "bounds are integers",
			min:    1,
			max:    33,
			powers: []float64{1, 2, 4, 8, 16, 32, 33},
		},
		{
			name:   "bounds are <1",
			min:    0.05,
			max:    0.5,
			powers: []float64{0.05, 0.0625, 0.125, 0.25, 0.5},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := powersOfTwoBetween(testCase.min, testCase.max), testCase.powers; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect powers between (%v,%v): %s", testCase.name, testCase.min, testCase.max, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestWriteHeader(t *testing.T) {
	testcases := []struct {
		name       string
		statusCode int
	}{
		{
			"StatusOK",
			http.StatusOK,
		},
		{
			"StatusNotFound",
			http.StatusNotFound,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			trw := &traceResponseWriter{ResponseWriter: rr, statusCode: http.StatusOK}
			trw.WriteHeader(tc.statusCode)
			if rr.Code != tc.statusCode {
				t.Errorf("mismatch in response headers: expected %s, got %s", http.StatusText(tc.statusCode), http.StatusText(rr.Code))
			}
			if trw.statusCode != tc.statusCode {
				t.Errorf("mismatch in TraceResponseWriter headers: expected %s, got %s", http.StatusText(tc.statusCode), http.StatusText(trw.statusCode))
			}
		})

	}

}

func TestWrite(t *testing.T) {
	testcases := []struct {
		name         string
		responseBody string
	}{
		{
			"SimpleText for trace",
			"Simple text to traceResponseWriter.size",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			trw := &traceResponseWriter{ResponseWriter: rr, statusCode: http.StatusOK}
			resp := []byte(tc.responseBody)
			_, err := trw.Write(resp)
			if err != nil {
				t.Fatalf("failed to write to TraceResponseWriter")
			}
			if rr.Body.String() != tc.responseBody {
				t.Errorf("mismatch in response body: expected %s, got %s", tc.responseBody, rr.Body.String())
			}
			if trw.size != len(resp) {
				t.Errorf("mismatch in TraceResponseWriter size: expected %d, got %d", len(resp), trw.size)
			}
		})
	}
}

func TestRecordError(t *testing.T) {
	testcases := []struct {
		name          string
		namespace     string
		expectedError string
		expectedCount int
		expectedOut   string
	}{
		{
			name:          "Simple Error String",
			namespace:     "testnamespace",
			expectedError: "sample error message to ensure proper working",
			expectedOut: `# HELP testnamespace_error_rate number of errors, sorted by label/type
					   # TYPE testnamespace_error_rate counter
					   testnamespace_error_rate{error="sample error message to ensure proper working"} 1
					   `,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			errorRate := ErrorRate(tc.namespace)
			RecordError(tc.expectedError, errorRate)
			if err := testutil.CollectAndCompare(errorRate, strings.NewReader(tc.expectedOut)); err != nil {
				t.Errorf("unexpected metrics for ErrorRate:\n%s", err)
			}

		})
	}
}

func oneByteWriter(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		oneByteLength := []byte{'1'}
		_, err := w.Write(oneByteLength)
		if err != nil {
			t.Fatalf("failed to write to TraceResponseWriter: %v", err)
		}
	}
}

func halfSecLatency(_ time.Time) time.Duration {
	return time.Millisecond * 500
}
func TestHandleWithMetricsCustomTimer(t *testing.T) {
	testcases := []struct {
		name                    string
		namespace               string
		customTimer             func(time.Time) time.Duration
		dummyWriter             http.HandlerFunc
		expectedResponseTimeOut string
		expectedResponseSizeOut string
	}{
		{
			name:        "Simple call to dummy handler with 0.5 sec latency and 1 byte response",
			namespace:   "testnamespace",
			customTimer: halfSecLatency,
			dummyWriter: oneByteWriter(t),
			expectedResponseTimeOut: `
            # HELP testnamespace_http_request_duration_seconds http request duration in seconds
            # TYPE testnamespace_http_request_duration_seconds histogram
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.0001"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.0001220703125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.000244140625"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.00048828125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.0009765625"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.001953125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.00390625"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.0078125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.015625"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.03125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.0625"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.125"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.25"} 0
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="0.5"} 1
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="1"} 1
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="2"} 1
            testnamespace_http_request_duration_seconds_bucket{method="GET",path="",status="200",user_agent="",le="+Inf"} 1
            testnamespace_http_request_duration_seconds_sum{method="GET",path="",status="200",user_agent=""} 0.5
            testnamespace_http_request_duration_seconds_count{method="GET",path="",status="200",user_agent=""} 1
			`,
			expectedResponseSizeOut: `
			# HELP testnamespace_http_response_size_bytes http response size in bytes
            # TYPE testnamespace_http_response_size_bytes histogram
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="256"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="512"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="1024"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="2048"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="4096"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="8192"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="16384"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="32768"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="65536"} 1
            testnamespace_http_response_size_bytes_bucket{method="GET",path="",status="200",user_agent="",le="+Inf"} 1
            testnamespace_http_response_size_bytes_sum{method="GET",path="",status="200",user_agent=""} 1
            testnamespace_http_response_size_bytes_count{method="GET",path="",status="200",user_agent=""} 1
			`,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			httpResponseSize := HttpResponseSize(tc.namespace, 256, 65536)
			httpRequestDuration := HttpRequestDuration(tc.namespace, 0.0001, 2)

			simplifier := simplifypath.NewSimplifier(simplifypath.L(""))
			handler := traceHandlerWithCustomTimer(simplifier, httpRequestDuration, httpResponseSize, tc.customTimer)(tc.dummyWriter)
			rr := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if err != nil {
				t.Errorf("error while creating dummy request: %w", err)
			}
			handler.ServeHTTP(rr, req)
			if err := testutil.CollectAndCompare(httpResponseSize, strings.NewReader(tc.expectedResponseSizeOut)); err != nil {
				t.Errorf("unexpected metrics for HTTPResponseSize:\n%s", err)
			}
			if err := testutil.CollectAndCompare(httpRequestDuration, strings.NewReader(tc.expectedResponseTimeOut)); err != nil {
				t.Errorf("unexpected metrics for HTTPRequestDuration:\n%s", err)
			}
		})
	}
}
