{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prow-stale',
        rules: [
          {
            alert: 'Prow images are stale',
            # Set day of week based stale alert, so that it can be stricter than 7 days, since k8s prow is automatically deployed now.
            # Considering that there might be days that there is no prow update(which might be rare but could be true), the alert should at least
            # be 2 work days. In considering weekends, monday and tuesdays will be +2 days.
            expr: |||
              ((time()-max(prow_version) > %d * 24 * 3600) and (day_of_week()<6) and (day_of_week()>2))
              or ((time()-max(prow_version) > %d * 24 * 3600) and (day_of_week()==1))
              or ((time()-max(prow_version) > %d * 24 * 3600) and (day_of_week()==2))
            ||| % [$._config.prowImageStaleByDays.daysStale, $._config.prowImageStaleByDays.daysStale+2, $._config.prowImageStaleByDays.daysStale+2],
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
