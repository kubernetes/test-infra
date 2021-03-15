{
  prometheusAlerts+:: {
    local componentName = $._config.components.monitoring,
    groups+: [
      {
        name: 'prow',
        rules: [
          {
            alert: 'prow-pod-crashlooping',
            expr: 'rate(kube_pod_container_status_restarts_total{namespace=~"default|prow-monitoring",job="kube-state-metrics"}[5m]) * 60 * 5 > 0',
            'for': '1m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              message: 'Pod {{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container}}) is restarting {{ printf "%.2f" $value }} times / 5 minutes.'
            },
          }
        ],
      },
    ],
  },
}
