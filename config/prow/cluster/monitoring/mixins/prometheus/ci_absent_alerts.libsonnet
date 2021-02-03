{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'ci-absent',
        rules: [
          {
            alert: '%sDown' % name,
            expr: |||
              absent(up{job="%s"} == 1)
            ||| % name,
            'for': '5m',
            labels: {
              severity: 'critical',
              slo: name,
            },
            annotations: {
              message: '@test-infra-oncall The service %s has been down for 5 minutes.' % name,
            },
          }
          for name in $._config.ciAbsents.components
        ],
      },
    ],
  },
}
