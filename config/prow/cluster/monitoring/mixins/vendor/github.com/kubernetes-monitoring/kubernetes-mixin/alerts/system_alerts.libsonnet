{
  _config+:: {
    notKubeDnsCoreDnsSelector: 'job!~"kube-dns|coredns"',
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-system',
        rules: [
          {
            alert: 'KubeVersionMismatch',
            expr: |||
              count(count by (gitVersion) (label_replace(kubernetes_build_info{%(notKubeDnsCoreDnsSelector)s},"gitVersion","$1","gitVersion","(v[0-9]*.[0-9]*).*"))) > 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'There are {{ $value }} different semantic versions of Kubernetes components running.',
              summary: 'Different semantic versions of Kubernetes components running.',
            },
          },
          {
            alert: 'KubeClientErrors',
            // Many clients use get requests to check the existence of objects,
            // this is normal and an expected error, therefore it should be
            // ignored in this alert.
            expr: |||
              (sum(rate(rest_client_requests_total{code=~"5.."}[5m])) by (instance, job)
                /
              sum(rate(rest_client_requests_total[5m])) by (instance, job))
              > 0.01
            |||,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: "Kubernetes API server client '{{ $labels.job }}/{{ $labels.instance }}' is experiencing {{ $value | humanizePercentage }} errors.'",
              summary: 'Kubernetes API server client is experiencing errors.',
            },
          },
        ],
      },
    ],
  },
}
