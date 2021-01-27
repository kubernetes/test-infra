{
  _config+:: {
    kubeStateMetricsSelector: error 'must provide selector for kube-state-metrics',
    nodeExporterSelector: error 'must provide selector for node-exporter',
    namespaceSelector: null,
    prefixedNamespaceSelector: if self.namespaceSelector != null then self.namespaceSelector + ',' else '',

    // We alert when the aggregate (CPU, Memory) quota for all namespaces is
    // greater than the amount of the resources in the cluster.  We do however
    // allow you to overcommit if you wish.
    namespaceOvercommitFactor: 1.5,
    cpuThrottlingPercent: 25,
    cpuThrottlingSelector: '',
    // Set this selector for seleting namespaces that contains resources used for overprovision
    // See https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-can-i-configure-overprovisioning-with-cluster-autoscaler
    // for more details.
    ignoringOverprovisionedWorkloadSelector: '',
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-resources',
        rules: [
          {
            alert: 'KubeCPUOvercommit',
            expr: |||
              sum(namespace:kube_pod_container_resource_requests_cpu_cores:sum{%(ignoringOverprovisionedWorkloadSelector)s})
                /
              sum(kube_node_status_allocatable_cpu_cores)
                >
              (count(kube_node_status_allocatable_cpu_cores)-1) / count(kube_node_status_allocatable_cpu_cores)
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Cluster has overcommitted CPU resource requests for Pods and cannot tolerate node failure.',
              summary: 'Cluster has overcommitted CPU resource requests.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeMemoryOvercommit',
            expr: |||
              sum(namespace:kube_pod_container_resource_requests_memory_bytes:sum{%(ignoringOverprovisionedWorkloadSelector)s})
                /
              sum(kube_node_status_allocatable_memory_bytes)
                >
              (count(kube_node_status_allocatable_memory_bytes)-1)
                /
              count(kube_node_status_allocatable_memory_bytes)
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Cluster has overcommitted memory resource requests for Pods and cannot tolerate node failure.',
              summary: 'Cluster has overcommitted memory resource requests.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeCPUQuotaOvercommit',
            expr: |||
              sum(kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard", resource="cpu"})
                /
              sum(kube_node_status_allocatable_cpu_cores)
                > %(namespaceOvercommitFactor)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Cluster has overcommitted CPU resource requests for Namespaces.',
              summary: 'Cluster has overcommitted CPU resource requests.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeMemoryQuotaOvercommit',
            expr: |||
              sum(kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard", resource="memory"})
                /
              sum(kube_node_status_allocatable_memory_bytes{%(nodeExporterSelector)s})
                > %(namespaceOvercommitFactor)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Cluster has overcommitted memory resource requests for Namespaces.',
              summary: 'Cluster has overcommitted memory resource requests.',
            },
            'for': '5m',
          },
          {
            alert: 'KubeQuotaFullyUsed',
            expr: |||
              kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="used"}
                / ignoring(instance, job, type)
              (kube_resourcequota{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, type="hard"} > 0)
                >= 1
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'info',
            },
            annotations: {
              description: 'Namespace {{ $labels.namespace }} is using {{ $value | humanizePercentage }} of its {{ $labels.resource }} quota.',
              summary: 'Namespace quota is fully used.',
            },
          },
          {
            alert: 'CPUThrottlingHigh',
            expr: |||
              sum(increase(container_cpu_cfs_throttled_periods_total{container!="", %(cpuThrottlingSelector)s}[5m])) by (container, pod, namespace)
                /
              sum(increase(container_cpu_cfs_periods_total{%(cpuThrottlingSelector)s}[5m])) by (container, pod, namespace)
                > ( %(cpuThrottlingPercent)s / 100 )
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'info',
            },
            annotations: {
              description: '{{ $value | humanizePercentage }} throttling of CPU in namespace {{ $labels.namespace }} for container {{ $labels.container }} in pod {{ $labels.pod }}.',
              summary: 'Processes experience elevated CPU throttling.',
            },
          },
        ],
      },
    ],
  },
}
