{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'prow-monitoring-absent',
        rules: [{
          alert: 'ServiceLostHA',
          expr: |||
            sum(up{job=~"prometheus|alertmanager"}) by (job) <= 1
          |||,
          'for': '5m',
          labels: { 
            severity: 'slack',
          },
          annotations: {
            message: 'The service {{ $labels.job }} has at most 1 instance for 5 minutes.',
          },
        }] + [
          {
            alert: '%sDown' % name,
            expr: |||
              absent(up{job="%s"} == 1)
            ||| % name,
            'for': '5m',
            labels: {
              severity: 'slack',
            },
            annotations: {
              message: 'The service %s has been down for 5 minutes.' % name,
            },
          }
          for name in ['alertmanager', 'prometheus', 'grafana']
        ],
      },
    ],
  },
}
