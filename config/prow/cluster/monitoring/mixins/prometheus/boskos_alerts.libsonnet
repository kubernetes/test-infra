{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'Boskos resource usage',
        rules: [
          {
            alert: 'Resource exhausted (0% free)',
            // This expression evaluates to true iff there is resource type with 0 'free' resources.
            // Resource pools with <= 5 resources are ignored since it is often expected for
            // small pools to use 100% capacity.
            expr: |||
              (
                min(boskos_resources{state="free"}) by (type, instance)
                and
                (sum(boskos_resources) by (type, instance) > 5)
              ) == 0
            |||,
            labels: {
              severity: 'high',
              'boskos_type': '{{ $labels.type }}',
            },
            annotations: {
              message: 'The Boskos resource "{{ $labels.type }}" has been exhausted (currently 0%% free). See the %s.' % [$._config.instance.monitoringLink('/d/wSrfvNxWz/boskos-resource-usage?orgId=1', 'Boskos resource usage dashboard')],
            },
          },
        ],
      },
    ],
  },
}
