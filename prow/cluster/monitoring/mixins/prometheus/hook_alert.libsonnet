{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'abnormal webhook behaviors',
        rules: [
          {
            alert: 'no-webhook-calls',
            // Monday-Friday 9am-5pm PDT (in UTC)
            expr: |||
              (sum(increase(prow_webhook_counter[1m])) == 0 or absent(prow_webhook_counter))
              and ( ((day_of_week() == 1) and (hour() >= 18)) or
                    ((day_of_week() > 2) and (day_of_week() < 6) and ((hour() >= 18) or (hour() <= 2))) or
                    ((day_of_week() == 6) and (hour() <= 2)) )
            |||,
            'for': '5m',
            labels: {
              severity: 'slack',
            },
            annotations: {
              message: 'There have been no webhook calls on working hours for 5 minutes',
            },
          },
        ],
      },
    ],
  },
}
