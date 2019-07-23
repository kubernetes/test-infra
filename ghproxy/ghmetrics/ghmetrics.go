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

package ghmetrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// ghTokenUntilResetGaugeVec provides the 'github_token_reset' gauge that
// enables keeping track of GitHub reset times.
var ghTokenUntilResetGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_token_reset",
		Help: "Last reported GitHub token reset time.",
	},
	[]string{"token_hash", "api_version"},
)

// ghTokenUsageGaugeVec provides the 'github_token_usage' gauge that
// enables keeping track of GitHub calls and quotas.
var ghTokenUsageGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_token_usage",
		Help: "How many GitHub token requets are remaining for the current hour.",
	},
	[]string{"token_hash", "api_version"},
)

var muxTokenUsage, muxRequestMetrics sync.Mutex
var lastGitHubResponse time.Time

func init() {
	prometheus.MustRegister(ghTokenUntilResetGaugeVec)
	prometheus.MustRegister(ghTokenUsageGaugeVec)
}

// CollectGitHubTokenMetrics publishes the rate limits of the github api to
// `github_token_usage` as well as `github_token_reset` on prometheus.
func CollectGitHubTokenMetrics(tokenHash, apiVersion string, headers http.Header, reqStartTime, responseTime time.Time) {
	remaining := headers.Get("X-RateLimit-Remaining")
	timeUntilReset := timestampStringToTime(headers.Get("X-RateLimit-Reset"))
	durationUntilReset := timeUntilReset.Sub(reqStartTime)

	remainingFloat, err := strconv.ParseFloat(remaining, 64)
	if err != nil {
		logrus.WithError(err).Infof("Couldn't convert number of remaining token requests into gauge value (float)")
	}

	muxTokenUsage.Lock()
	isAfter := lastGitHubResponse.After(responseTime)
	if !isAfter {
		lastGitHubResponse = responseTime
	}
	muxTokenUsage.Unlock()
	if isAfter {
		logrus.WithField("last-github-response", lastGitHubResponse).WithField("response-time", responseTime).Debug("Previously pushed metrics of a newer response, skipping old metrics")
	} else {
		ghTokenUntilResetGaugeVec.With(prometheus.Labels{"token_hash": tokenHash, "api_version": apiVersion}).Set(float64(durationUntilReset.Nanoseconds()))
		ghTokenUsageGaugeVec.With(prometheus.Labels{"token_hash": tokenHash, "api_version": apiVersion}).Set(remainingFloat)
	}
}

// timestampStringToTime takes a unix timestamp and returns a `time.Time`
// from the given time.
func timestampStringToTime(tstamp string) time.Time {
	timestamp, err := strconv.ParseInt(tstamp, 10, 64)
	if err != nil {
		logrus.WithField("timestamp", tstamp).Info("Couldn't convert unix timestamp")
	}
	return time.Unix(timestamp, 0)
}
