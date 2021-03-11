{
  prometheusAlerts+:: {
    local monitoringLink = $._config.instance.monitoringLink,
    local dashboardID = $._config.grafanaDashboardIDs['ghproxy.json'],
    groups+: [
      {
        name: 'ghproxy',
        rules: [
          {
            alert: 'ghproxy-specific-status-code-5xx',
            expr: |||
              sum(rate(github_request_duration_count{status=~"5.."}[5m])) by (status,path) / ignoring(status) group_left sum(rate(github_request_duration_count[5m])) by (path) * 100 > 10
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all requests for {{ $labels.path }} through the GitHub proxy are erroring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=9' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-global-status-code-5xx',
            expr: |||
              sum(rate(github_request_duration_count{status=~"5.."}[5m])) by (status) / ignoring(status) group_left sum(rate(github_request_duration_count[5m])) * 100 > 3
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all API requests through the GitHub proxy are errorring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=8' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-specific-status-code-4xx',
            // Paths that contains error codes expected by prow(Grabbed from previous prow alerts):
            //  - "/repos/:owner/:repo/pulls/:pullId/requested_reviewers" 422 (https://github.com/kubernetes/test-infra/blob/e84a6897b7fae65ba295a4c370057e4a216345ef/prow/github/client.go#L2712)
            //  - "/search/issues" 403 (Permission denied, very likely not prow error)
            //  - "/repos/:owner/:repo/pulls/:pullId/merge" 405 (https://github.com/kubernetes/test-infra/blob/e84a6897b7fae65ba295a4c370057e4a216345ef/prow/github/client.go#L3472)
            //  These paths + statuscode combinations are excluded from alerts to reduce noise.
            expr: |||
               sum by(status, path) (rate(github_request_duration_count{status!="404",status!="410",status=~"4..",path!="/repos/:owner/:repo/pulls/:pullId/requested_reviewers",path!="/search/issues",path!="/repos/:owner/:repo/pulls/:pullId/merge",path!="/repos/:owner/:repo/statuses/:statusId"}[30m])) / ignoring(status) group_left() sum by(path) (rate(github_request_duration_count[30m])) * 100 > 10
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all requests for {{ $labels.path }} through the GitHub proxy are erroring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=9' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-specific-status-code-not-422',
            expr: |||
               sum by(status, path) (rate(github_request_duration_count{status!="404",status!="410", status!="422", status=~"4..",path="/repos/:owner/:repo/pulls/:pullId/requested_reviewers"}[30m])) / ignoring(status) group_left() sum by(path) (rate(github_request_duration_count[30m])) * 100 > 10
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all requests for {{ $labels.path }} through the GitHub proxy are erroring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=9' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-specific-status-code-not-403',
            expr: |||
               sum by(status, path) (rate(github_request_duration_count{status!="404",status!="410", status!="403", status=~"4..",path="/search/issues"}[30m])) / ignoring(status) group_left() sum by(path) (rate(github_request_duration_count[30m])) * 100 > 10
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all requests for {{ $labels.path }} through the GitHub proxy are erroring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=9' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-specific-status-code-not-405',
            expr: |||
               sum by(status, path) (rate(github_request_duration_count{status!="404",status!="410", status!="405", status=~"4..",path="/repos/:owner/:repo/pulls/:pullId/merge"}[30m])) / ignoring(status) group_left() sum by(path) (rate(github_request_duration_count[30m])) * 100 > 10
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all requests for {{ $labels.path }} through the GitHub proxy are erroring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=9' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-global-status-code-4xx',
            expr: |||
              sum(rate(github_request_duration_count{status=~"4..",status!="404",status!="410"}[30m])) by (status) / ignoring(status) group_left sum(rate(github_request_duration_count[30m])) * 100 > 3
            |||,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ $value | humanize }}%% of all API requests through the GitHub proxy are errorring with code {{ $labels.status }}. Check %s.' % [monitoringLink('/d/%s/github-cache?orgId=1&refresh=1m&fullscreen&panelId=8' % [dashboardID], 'the ghproxy dashboard')],
            },
          },
          {
            alert: 'ghproxy-running-out-github-tokens-in-a-hour',
            // check 30% of the capacity (5000): 1500
            expr: |||
              github_token_usage{job="ghproxy"} <  1500
              and
              predict_linear(github_token_usage{job="ghproxy"}[30m], 1 * 3600) < 0
            |||,
            'for': '5m',
            labels: {
              severity: 'high',
            },
            annotations: {
              message: 'token {{ $labels.token_hash }} will run out of API quota before the next reset.',
            },
          }
        ],
      },
    ],
  },
}
