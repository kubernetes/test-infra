{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'Boskos resource usage',
        rules: [
          {
            alert: 'Low resource availability (<10% free)',
            // This expression calculates the percentage of resources of each type that are free.
            // If there are multiple instances the most pessimistic value is used.
            // The threshold for the alert is <10% free.
            // Resource pools with <= 5 resources are ignored since it is often expected for
            // small pools to use 100% capacity.
            expr: |||
              min(
                min(boskos_resources{state="free"}) by (type, instance)
                /
                (sum(boskos_resources) by (type, instance) > 5)
              ) by (type) * 100
              < 10
            |||,
            labels: {
              severity: 'warning',
              'boskos-type': '{{ $labels.type }}',
            },
            annotations: {
              message: 'The Boskos resource "{{ $labels.type }}" has low availability (currently {{ printf "%0.2f" $value }}% free). See the <https://monitoring.prow.k8s.io/d/wSrfvNxWz/boskos-resource-usage?orgId=1|Boskos resource usage dashboard>.',
            },
          },
        ],
      },
    ],
  },
}
