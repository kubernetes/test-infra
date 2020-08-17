{
  prometheusRules+:: {
    groups+: [
      {
        name: 'windows.node.rules',
        rules: [
          {
            // This rule gives the number of windows nodes
            record: 'node:windows_node:sum',
            expr: |||
              count (
                wmi_system_system_up_time{%(wmiExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            // This rule gives the number of CPUs per node.
            record: 'node:windows_node_num_cpu:sum',
            expr: |||
              count by (instance) (sum by (instance, core) (
                wmi_cpu_time_total{%(wmiExporterSelector)s}
              ))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: ':windows_node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg(rate(wmi_cpu_time_total{%(wmiExporterSelector)s,mode="idle"}[1m]))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: 'node:windows_node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg by (instance) (
                rate(wmi_cpu_time_total{%(wmiExporterSelector)s,mode="idle"}[1m])
              )
            ||| % $._config,
          },
          {
            record: ':windows_node_memory_utilisation:',
            expr: |||
              1 -
              sum(wmi_memory_available_bytes{%(wmiExporterSelector)s})
              /
              sum(wmi_os_visible_memory_bytes{%(wmiExporterSelector)s})
            ||| % $._config,
          },
          // Add separate rules for Free & Total, so we can aggregate across clusters
          // in dashboards.
          {
            record: ':windows_node_memory_MemFreeCached_bytes:sum',
            expr: |||
              sum(wmi_memory_available_bytes{%(wmiExporterSelector)s} + wmi_memory_cache_bytes{%(wmiExporterSelector)s})
            ||| % $._config,
          },
          {
            record: 'node:windows_node_memory_totalCached_bytes:sum',
            expr: |||
              (wmi_memory_cache_bytes{%(wmiExporterSelector)s} + wmi_memory_modified_page_list_bytes{%(wmiExporterSelector)s} + wmi_memory_standby_cache_core_bytes{%(wmiExporterSelector)s} + wmi_memory_standby_cache_normal_priority_bytes{%(wmiExporterSelector)s} + wmi_memory_standby_cache_reserve_bytes{%(wmiExporterSelector)s})
            ||| % $._config,
          },
          {
            record: ':windows_node_memory_MemTotal_bytes:sum',
            expr: |||
              sum(wmi_os_visible_memory_bytes{%(wmiExporterSelector)s})
            ||| % $._config,
          },
          {
            // Available memory per node
            // SINCE 2018-02-08
            record: 'node:windows_node_memory_bytes_available:sum',
            expr: |||
              sum by (instance) (
                (wmi_memory_available_bytes{%(wmiExporterSelector)s})
              )
            ||| % $._config,
          },
          {
            // Total memory per node
            record: 'node:windows_node_memory_bytes_total:sum',
            expr: |||
              sum by (instance) (
                wmi_os_visible_memory_bytes{%(wmiExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            // Memory utilisation per node, normalized by per-node memory
            record: 'node:windows_node_memory_utilisation:ratio',
            expr: |||
              (node:windows_node_memory_bytes_total:sum - node:windows_node_memory_bytes_available:sum)
              /
              scalar(sum(node:windows_node_memory_bytes_total:sum))
            |||,
          },
          {
            record: 'node:windows_node_memory_utilisation:',
            expr: |||
              1 - (node:windows_node_memory_bytes_available:sum / node:windows_node_memory_bytes_total:sum)
            ||| % $._config,
          },
          {
            record: 'node:windows_node_memory_swap_io_pages:irate',
            expr: |||
              irate(wmi_memory_swap_page_operations_total{%(wmiExporterSelector)s}[5m])
            ||| % $._config,
          },
          {
            // Disk utilisation (ms spent, by rate() it's bound by 1 second)
            record: ':windows_node_disk_utilisation:avg_irate',
            expr: |||
              avg(irate(wmi_logical_disk_read_seconds_total{%(wmiExporterSelector)s}[1m]) + 
                  irate(wmi_logical_disk_write_seconds_total{%(wmiExporterSelector)s}[1m])
                )
            ||| % $._config,
          },
          {
            // Disk utilisation (ms spent, by rate() it's bound by 1 second)
            record: 'node:windows_node_disk_utilisation:avg_irate',
            expr: |||
              avg by (instance) (
                (irate(wmi_logical_disk_read_seconds_total{%(wmiExporterSelector)s}[1m]) +
                 irate(wmi_logical_disk_write_seconds_total{%(wmiExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
          {
            record: 'node:windows_node_filesystem_usage:',
            expr: |||
              max by (instance,volume)(
                (wmi_logical_disk_size_bytes{%(wmiExporterSelector)s}
              - wmi_logical_disk_free_bytes{%(wmiExporterSelector)s})
              / wmi_logical_disk_size_bytes{%(wmiExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            record: 'node:windows_node_filesystem_avail:',
            expr: |||
              max by (instance, volume) (wmi_logical_disk_free_bytes{%(wmiExporterSelector)s} / wmi_logical_disk_size_bytes{%(wmiExporterSelector)s})
            ||| % $._config,
          },
          {
            record: ':windows_node_net_utilisation:sum_irate',
            expr: |||
              sum(irate(wmi_net_bytes_total{%(wmiExporterSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:windows_node_net_utilisation:sum_irate',
            expr: |||
              sum by (instance) (
                (irate(wmi_net_bytes_total{%(wmiExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
          {
            record: ':windows_node_net_saturation:sum_irate',
            expr: |||
              sum(irate(wmi_net_packets_received_discarded{%(wmiExporterSelector)s}[1m])) +
              sum(irate(wmi_net_packets_outbound_discarded{%(wmiExporterSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:windows_node_net_saturation:sum_irate',
            expr: |||
              sum by (instance) (
                (irate(wmi_net_packets_received_discarded{%(wmiExporterSelector)s}[1m]) +
                irate(wmi_net_packets_outbound_discarded{%(wmiExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
        ],
      },
      {
        name: 'windows.pod.rules',
        rules: [
          {
            record: 'windows_container_available',
            expr: |||
              wmi_container_available{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_total_runtime',
            expr: |||
              wmi_container_cpu_usage_seconds_total{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_memory_usage',
            expr: |||
              wmi_container_memory_usage_commit_bytes{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_private_working_set_usage',
            expr: |||
              wmi_container_memory_usage_private_working_set_bytes{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_network_receive_bytes_total',
            expr: |||
              wmi_container_network_receive_bytes_total{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_network_transmit_bytes_total',
            expr: |||
              wmi_container_network_transmit_bytes_total{%(wmiExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_memory_request',
            expr: |||
              kube_pod_container_resource_requests_memory_bytes {%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_memory_limit',
            expr: |||
              kube_pod_container_resource_limits_memory_bytes {%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_cpu_cores_request',
            expr: |||
              kube_pod_container_resource_requests_cpu_cores  {%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_cpu_cores_limit',
            expr: |||
              kube_pod_container_resource_limits_cpu_cores  {%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_container_available)
            ||| % $._config,
          },
          {
            record: 'namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate',
            expr: |||
              sum by (namespace, pod, container) (
                rate(windows_container_total_runtime{}[5m])
              )
            ||| % $._config,
          },
        ],
      },
    ],
  },
}
