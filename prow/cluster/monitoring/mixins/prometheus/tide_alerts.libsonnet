{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'Tide progress',
        rules: [
          {
            alert: 'TideNoProgressKK',
            expr: |||
              clamp_min(
              (sum(merges_sum{org="kubernetes",repo="kubernetes",branch="master"}) or vector(0))
               - (sum(merges_sum{org="kubernetes",repo="kubernetes",branch="master"} offset 5m ) or vector(0)),
              0) < 0.5
              and
              max(min_over_time(pooledprs{org="kubernetes",repo="kubernetes",branch="master"}[4h])) > 0.5
            |||,
            'for': '4h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Tide has not merged any PRs in kubernetes/kubernetes:master in the past 4 hours despite PRs in the pool.',
            },
          },
          {
            alert: 'TideSyncLoopDuration',
            expr: |||
              avg_over_time(syncdur{job="tide"}[15m]) > 120
            |||,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'The Tide sync controllers loop period has averaged more than 2 minutes for the last 15 mins.',
            },
          },
          {
            alert: 'TideStatusUpdateLoopDuration',
            expr: |||
              avg_over_time(statusupdatedur{job="tide"}[15m]) > 120
            |||,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'The Tide status update controllers loop period has averaged more than 2 minutes for the last 15 mins.',
            },
          },
        ],
      },
    ],
  },
}
