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
            },
            annotations: {
              message: '@test-infra-oncall The service %s has been down for 5 minutes.' % name,
            },
          }
          for name in ['deck', 'ghproxy', 'hook', 'plank', 'sinker', 'tide']
        ],
      },
    ],
  },
}
