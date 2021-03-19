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
            'for': $._config.prowImageStaleByDays.eventDuration,
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'The prow images are older than %(daysStale)d days for %(eventDuration)s.' % ($._config.prowImageStaleByDays),
            },
          }
        ],
      },
    ],
  },
}
