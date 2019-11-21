{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'sinker-missing',
        rules: [
          {
            alert: 'SinkerNotRemovingPods',
            expr: |||
              absent(sum(rate(sinker_pods_removed[1h]))) == 1
            |||,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Sinker has not removed any Pods in the last hour, likely indicating an outage in the service.',
            },
          },
          {
            alert: 'SinkerNotRemovingProwJobs',
            expr: |||
              absent(sum(rate(sinker_prow_jobs_cleaned[1h]))) == 1
            |||,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Sinker has not removed any Prow jobs in the last hour, likely indicating an outage in the service.',
            },
          }
        ],
      },
    ],
  },
}