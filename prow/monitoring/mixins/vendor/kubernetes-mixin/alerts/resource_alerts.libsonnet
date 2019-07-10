{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-resources',
        rules: [
          {
            alert: 'KubeCPUOvercommit',
            expr: |||
              sum(namespace:kube_pod_container_resource_requests_cpu_cores:sum)
                /
              sum(node:node_num_cpu:sum)
                >
              (count(node:node_num_cpu:sum)-1) / count(node:node_num_cpu:sum)
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Cluster has overcommitted CPU resource requests for Pods and cannot tolerate node failure.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeMemOvercommit',
            expr: |||
              sum(namespace:kube_pod_container_resource_requests_memory_bytes:sum)
                /
              sum(node_memory_MemTotal_bytes)
                >
              (count(node:node_num_cpu:sum)-1)
                /
              count(node:node_num_cpu:sum)
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Cluster has overcommitted memory resource requests for Pods and cannot tolerate node failure.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeCPUOvercommit',
            expr: |||
              sum(kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard", resource="cpu"})
                /
              sum(node:node_num_cpu:sum)
                > %(namespaceOvercommitFactor)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Cluster has overcommitted CPU resource requests for Namespaces.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeMemOvercommit',
            expr: |||
              sum(kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard", resource="memory"})
                /
              sum(node_memory_MemTotal_bytes{%(nodeExporterSelector)s})
                > %(namespaceOvercommitFactor)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Cluster has overcommitted memory resource requests for Namespaces.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeQuotaExceeded',
            expr: |||
              100 * kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="used"}
                / ignoring(instance, job, type)
              (kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard"} > 0)
                > 90
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'Namespace {{ $labels.namespace }} is using {{ printf "%0.0f" $value }}% of its {{ $labels.resource }} quota.',
            },
          },
          {
            alert: 'CPUThrottlingHigh',
            expr: |||
              100 * sum(increase(container_cpu_cfs_throttled_periods_total{container!="", %(cpuThrottlingSelector)s}[5m])) by (container, pod, namespace)
                /
              sum(increase(container_cpu_cfs_periods_total{%(cpuThrottlingSelector)s}[5m])) by (container, pod, namespace)
                > %(cpuThrottlingPercent)s 
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: '{{ printf "%0.0f" $value }}% throttling of CPU in namespace {{ $labels.namespace }} for container {{ $labels.container }} in pod {{ $labels.pod }}.',
            },
          },
        ],
      },
    ],
  },
}
