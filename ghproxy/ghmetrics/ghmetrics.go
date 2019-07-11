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
	[]string{"token_hash", "until_reset"},
)

// ghTokenUsageGaugeVec provides the 'github_token_usage' gauge that
// enables keeping track of GitHub calls and quotas.
var ghTokenUsageGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_token_usage",
		Help: "How many GitHub token requets have been used up.",
	},
	[]string{"token_hash", "remaining"},
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

func init() {
	prometheus.MustRegister(ghTokenUntilResetGaugeVec)
	prometheus.MustRegister(ghTokenUsageGaugeVec)
	prometheus.MustRegister(ghRequestsGauge)
}

// CollectGithubTokenMetrics publishes the rate limits of the github api to
// `github_token_usage` on prometheus.
func CollectGithubTokenMetrics(tokenHash string, headers http.Header, now time.Time) {
	remaining := headers.Get("X-RateLimit-Remaining")
	timeUntilReset := timeUntilFromUnix(headers.Get("X-RateLimit-Reset"), now)

	remainingFloat, err := strconv.ParseFloat(remaining, 64)
	if err != nil {
		logrus.WithError(err).Warningf("Couldn't convert number of remaining token requests into gauge value (float)")
	}

	muxTokenUsage.Lock()
	defer muxTokenUsage.Unlock()
	ghTokenUntilResetGaugeVec.With(prometheus.Labels{"token_hash": tokenHash, "until_reset": timeUntilReset.String()}).Set(float64(timeUntilReset.Nanoseconds()))
	ghTokenUsageGaugeVec.With(prometheus.Labels{"token_hash": tokenHash, "remaining": remaining}).Set(remainingFloat)
}

// CollectGithubRequestMetrics publishes the number of requests by API path to
// `github_requests` on prometheus.
func CollectGithubRequestMetrics(tokenHash, path, statusCode, roundTripTime string) {
	muxRequestMetrics.Lock()
	defer muxRequestMetrics.Unlock()
	ghRequestsGauge.With(prometheus.Labels{"token_hash": tokenHash, "path": getSimplifiedPath(path), "status": statusCode, "duration": roundTripTime}).Inc()
}

// timeUntilFromUnix takes a unix timestamp and returns a `time.Duration`
// from the given time until the passed unix timestamps point in time.
func timeUntilFromUnix(reset string, now time.Time) time.Duration {
	timestamp, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		logrus.WithField("timestamp", reset).Info("Couldn't convert unix timestamp")
	}
	resetTime := time.Unix(timestamp, 0)
	return resetTime.Sub(now)
}
