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
	[]string{"token_hash"},
)

// ghTokenUsageGaugeVec provides the 'github_token_usage' gauge that
// enables keeping track of GitHub calls and quotas.
var ghTokenUsageGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_token_usage",
		Help: "How many GitHub token requets have been used up.",
	},
	[]string{"token_hash"},
)

// ghRequestsGauge provides the 'github_requests' gauge that keeps track
// of the number of GitHub requests by API path.
var ghRequestsGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_requests",
		Help: "GitHub requests by API path.",
	},
	[]string{"token_hash", "path", "status", "duration"},
)

var muxTokenUsage, muxRequestMetrics sync.Mutex
var lastGitHubResponse time.Time

func init() {
	prometheus.MustRegister(ghTokenUntilResetGaugeVec)
	prometheus.MustRegister(ghTokenUsageGaugeVec)
	prometheus.MustRegister(ghRequestsGauge)
}

// CollectGithubTokenMetrics publishes the rate limits of the github api to
// `github_token_usage` on prometheus.
func CollectGithubTokenMetrics(tokenHash string, headers http.Header, reqStartTime, responseTime time.Time) {
	remaining := headers.Get("X-RateLimit-Remaining")
	timeUntilReset := timestampStringToTime(headers.Get("X-RateLimit-Reset"))
	durationUntilReset := timeUntilReset.Sub(reqStartTime)

	remainingFloat, err := strconv.ParseFloat(remaining, 64)
	if err != nil {
		logrus.WithError(err).Infof("Couldn't convert number of remaining token requests into gauge value (float)")
	}

	muxTokenUsage.Lock()
	defer muxTokenUsage.Unlock()
	if lastGitHubResponse.After(responseTime) {
		logrus.WithField("lastGitHubResponse", lastGitHubResponse).WithField("responseTime", responseTime).Info("Previously pushed metrics of a newer response, skipping old metrics")
	} else {
		lastGitHubResponse = responseTime
		ghTokenUntilResetGaugeVec.With(prometheus.Labels{"token_hash": tokenHash}).Set(float64(durationUntilReset.Nanoseconds()))
		ghTokenUsageGaugeVec.With(prometheus.Labels{"token_hash": tokenHash}).Set(remainingFloat)
	}
}

// CollectGithubRequestMetrics publishes the number of requests by API path to
// `github_requests` on prometheus.
func CollectGithubRequestMetrics(tokenHash, path, statusCode, roundTripTime string) {
	muxRequestMetrics.Lock()
	defer muxRequestMetrics.Unlock()
	ghRequestsGauge.With(prometheus.Labels{"token_hash": tokenHash, "path": getSimplifiedPath(path), "status": statusCode, "duration": roundTripTime}).Inc()
}

// timeUntilFromUnix takes a unix timestamp and returns a `time.Time`
// from the given time until the passed unix timestamps point in time.
func timestampStringToTime(futureTimestamp string) time.Time {
	timestamp, err := strconv.ParseInt(futureTimestamp, 10, 64)
	if err != nil {
		logrus.WithField("timestamp", futureTimestamp).Info("Couldn't convert unix timestamp")
	}
	return time.Unix(timestamp, 0)
}
