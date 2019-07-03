package ghmetrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// ghTokenUsageGaugeVec provides the 'github_token_usage' gauge that
// enables keeping track of GitHub calls and quotas.
var ghTokenUsageGaugeVec = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_token_usage",
		Help: "How many GitHub token requets have been used up.",
	},
	[]string{"remaining", "until_reset"},
)

// ghRequestsGauge provides the 'github_requests' gauge that keeps track
// of the number of GitHub requests by API path.
var ghRequestsGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_requests",
		Help: "GitHub requests by API path.",
	},
	[]string{"path", "status", "duration"},
)

func init() {
	prometheus.MustRegister(ghTokenUsageGaugeVec)
	prometheus.MustRegister(ghRequestsGauge)
}

// GithubTokenMetrics publishes the rate limits of the github api to
// `github_token_usage` on prometheus.
func GithubTokenMetrics(headers http.Header, now time.Time) {
	remaining := headers.Get("X-RateLimit-Remaining")
	timeUntilReset := timeUntilFromUnix(headers.Get("X-RateLimit-Reset"), now)

	ghTokenUsageGaugeVec.With(prometheus.Labels{"remaining": remaining, "until_reset": timeUntilReset.String()}).Inc()
}

// GithubRequestMetrics publishes the number of requests by API path to
// `github_requests` on prometheus.
func GithubRequestMetrics(path, statusCode, roundTripTime string) {
	ghRequestsGauge.With(prometheus.Labels{"path": getSimplifiedPath(path), "status": statusCode, "duration": roundTripTime}).Inc()
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
