{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'ghproxy',
        rules: [
          {
            alert: 'ghproxy-status-code-abnormal-4XX',
            // excluding 404 because it does not indicate any error in the system
            expr: |||
              sum(rate(github_request_duration_count{status=~"4..", status!="404"}[5m])) / sum(rate(github_request_duration_count{status!="404"}[5m])) * 100 > 20
            |||,
            labels: {
              severity: 'slack',
            },
            annotations: {
              message: 'ghproxy has {{ $value | humanize }}%% of status code 4XX in the last 5 minutes.',
            },
          }
        ] + [
          {
            alert: 'ghproxy-status-code-abnormal-5XX',
            expr: |||
              sum(rate(github_request_duration_count{status=~"5.."}[5m])) / sum(rate(github_request_duration_count[5m])) * 100 > 5
            |||,
            labels: {
              severity: 'slack',
            },
            annotations: {
              message: 'ghproxy has {{ $value | humanize }}%% of status code 5XX in the last 5 minutes.',
            },
          }
        ] + [
          {
            alert: 'ghproxy-running-out-github-tokens-in-a-hour',
            // check 30% of the capacity (5000): 1500
            expr: |||
              github_token_usage{job="ghproxy"} <  1500
              and
              predict_linear(github_token_usage{job="ghproxy"}[1h], 1 * 3600) < 0
            |||,
            'for': '5m',
            labels: {
              severity: 'slack',
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
