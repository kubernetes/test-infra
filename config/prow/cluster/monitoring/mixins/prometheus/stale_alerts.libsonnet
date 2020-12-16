{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prow-stale',
        rules: [
          {
            alert: 'Prow images are stale',
            expr: |||
              time()-max(prow_version) > %d * 24 * 3600
            ||| % $._config.prowImageStaleByDays.daysStale,
            'for': '%s' % $._config.prowImageStaleByDays.hoursToTrigger,
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: '@test-infra-oncall The prow images are older than %d days.' % $._config.prowImageStaleByDays.daysStale,
            },
          }
        ],
      },
    ],
  },
}
