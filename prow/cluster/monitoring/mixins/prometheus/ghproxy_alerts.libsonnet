{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'ghproxy',
        rules: [
          {
            alert: 'ghproxy-status-code-abnormal-%sXX' % code_prefix,
            // excluding 404 because it does not indicate any error in the system
            expr: |||
              sum(rate(github_request_duration_count{status=~"%s..", status!="404"}[5m])) / sum(rate(github_request_duration_count{status!="404"}[5m])) * 100 > 5
            ||| % code_prefix,
            labels: {
              severity: 'slack',
            },
            annotations: {
              message: 'ghproxy has {{ $value | humanize }}%% of status code %sXX in the last 5 minutes.' % code_prefix,
            },
          }
          for code_prefix in ['4', '5']
        ] +
        [
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
