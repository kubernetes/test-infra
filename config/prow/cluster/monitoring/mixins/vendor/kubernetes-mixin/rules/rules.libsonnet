{
  prometheusRules+:: {
    groups+: [
      {
        name: 'k8s.rules',
        rules: [
          {
            record: 'namespace:container_cpu_usage_seconds_total:sum_rate',
            expr: |||
              sum(rate(container_cpu_usage_seconds_total{%(cadvisorSelector)s, image!="", container!="POD"}[5m])) by (namespace)
            ||| % $._config,
          },
          {
            // Reduces cardinality of this timeseries by #cores, which makes it
            // more useable in dashboards.  Also, allows us to do things like
            // quantile_over_time(...) which would otherwise not be possible.
            record: 'namespace_pod_container:container_cpu_usage_seconds_total:sum_rate',
            expr: |||
              sum by (namespace, pod, container) (
                rate(container_cpu_usage_seconds_total{%(cadvisorSelector)s, image!="", container!="POD"}[5m])
              )
            ||| % $._config,
          },
          {
            record: 'namespace:container_memory_usage_bytes:sum',
            expr: |||
              sum(container_memory_usage_bytes{%(cadvisorSelector)s, image!="", container!="POD"}) by (namespace)
            ||| % $._config,
          },
          {
            record: 'namespace:container_cpu_usage_seconds_total:sum_rate',
            expr: |||
              sum by (namespace, label_name) (
                  sum(rate(container_cpu_usage_seconds_total{%(cadvisorSelector)s, image!="", container!="POD"}[5m])) by (namespace, pod)
                * on (namespace, pod)
                  group_left(label_name) kube_pod_labels{%(kubeStateMetricsSelector)s}
              )
            ||| % $._config,
          },
          {
            record: 'namespace:container_memory_usage_bytes:sum',
            expr: |||
              sum by (namespace, label_name) (
                  sum(container_memory_usage_bytes{%(cadvisorSelector)s,image!="", container!="POD"}) by (pod, namespace)
                * on (namespace, pod)
                  group_left(label_name) kube_pod_labels{%(kubeStateMetricsSelector)s}
              )
            ||| % $._config,
          },
          {
            record: 'namespace:kube_pod_container_resource_requests_memory_bytes:sum',
            expr: |||
              sum by (namespace, label_name) (
                  sum(kube_pod_container_resource_requests_memory_bytes{%(kubeStateMetricsSelector)s} * on (endpoint, instance, job, namespace, pod, service) group_left(phase) (kube_pod_status_phase{phase=~"^(Pending|Running)$"} == 1)) by (namespace, pod)
                * on (namespace, pod)
                  group_left(label_name) kube_pod_labels{%(kubeStateMetricsSelector)s}
              )
            ||| % $._config,
          },
          {
            record: 'namespace:kube_pod_container_resource_requests_cpu_cores:sum',
            expr: |||
              sum by (namespace, label_name) (
                  sum(kube_pod_container_resource_requests_cpu_cores{%(kubeStateMetricsSelector)s} * on (endpoint, instance, job, namespace, pod, service) group_left(phase) (kube_pod_status_phase{phase=~"^(Pending|Running)$"} == 1)) by (namespace, pod)
                * on (namespace, pod)
                  group_left(label_name) kube_pod_labels{%(kubeStateMetricsSelector)s}
              )
            ||| % $._config,
          },
          // workload aggregation for deployments
          {
            record: 'mixin_pod_workload',
            expr: |||
              sum(
                label_replace(
                  label_replace(
                    kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="ReplicaSet"},
                    "replicaset", "$1", "owner_name", "(.*)"
                  ) * on(replicaset, namespace) group_left(owner_name) kube_replicaset_owner{%(kubeStateMetricsSelector)s},
                  "workload", "$1", "owner_name", "(.*)"
                )
              ) by (namespace, workload, pod)
            ||| % $._config,
            labels: {
              workload_type: 'deployment',
            },
          },
          {
            record: 'mixin_pod_workload',
            expr: |||
              sum(
                label_replace(
                  kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="DaemonSet"},
                  "workload", "$1", "owner_name", "(.*)"
                )
              ) by (namespace, workload, pod)
            ||| % $._config,
            labels: {
              workload_type: 'daemonset',
            },
          },
          {
            record: 'mixin_pod_workload',
            expr: |||
              sum(
                label_replace(
                  kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="StatefulSet"},
                  "workload", "$1", "owner_name", "(.*)"
                )
              ) by (namespace, workload, pod)
            ||| % $._config,
            labels: {
              workload_type: 'statefulset',
            },
          },
        ],
      },
      {
        name: 'kube-scheduler.rules',
        rules: [
          {
            record: 'cluster_quantile:%s:histogram_quantile' % metric,
            expr: |||
              histogram_quantile(%(quantile)s, sum(rate(%(metric)s_bucket{%(kubeSchedulerSelector)s}[5m])) without(instance, %(podLabel)s))
            ||| % ({ quantile: quantile, metric: metric } + $._config),
            labels: {
              quantile: quantile,
            },
          }
          for quantile in ['0.99', '0.9', '0.5']
          for metric in [
            'scheduler_e2e_scheduling_duration_seconds',
            'scheduler_scheduling_algorithm_duration_seconds',
            'scheduler_binding_duration_seconds',
          ]
        ],
      },
      {
        name: 'kube-apiserver.rules',
        rules: [
          {
            record: 'cluster_quantile:apiserver_request_duration_seconds:histogram_quantile',
            expr: |||
              histogram_quantile(%(quantile)s, sum(rate(apiserver_request_duration_seconds_bucket{%(kubeApiserverSelector)s}[5m])) without(instance, %(podLabel)s))
            ||| % ({ quantile: quantile } + $._config),
            labels: {
              quantile: quantile,
            },
          }
          for quantile in ['0.99', '0.9', '0.5']
        ],
      },
      {
        name: 'node.rules',
        rules: [
          {
            // Number of nodes in the cluster
            // SINCE 2018-02-08
            record: ':kube_pod_info_node_count:',
            expr: 'sum(min(kube_pod_info) by (node))',
          },
          {
            // This rule results in the tuples (node, namespace, instance) => 1;
            // it is used to calculate per-node metrics, given namespace & instance.
            record: 'node_namespace_pod:kube_pod_info:',
            expr: |||
              max(label_replace(kube_pod_info{%(kubeStateMetricsSelector)s}, "%(podLabel)s", "$1", "pod", "(.*)")) by (node, namespace, %(podLabel)s)
            ||| % $._config,
          },
          {
            // This rule gives the number of CPUs per node.
            record: 'node:node_num_cpu:sum',
            expr: |||
              count by (node) (sum by (node, cpu) (
                node_cpu_seconds_total{%(nodeExporterSelector)s}
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              ))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: ':node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg(rate(node_cpu_seconds_total{%(nodeExporterSelector)s,mode="idle"}[1m]))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: 'node:node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg by (node) (
                rate(node_cpu_seconds_total{%(nodeExporterSelector)s,mode="idle"}[1m])
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:)
            ||| % $._config,
          },
          {
            // CPU utilisation per node, normalized by cluster-wide CPUs
            record: 'node:cluster_cpu_utilisation:ratio',
            expr: |||
              node:node_cpu_utilisation:avg1m
                *
              node:node_num_cpu:sum
                /
              scalar(sum(node:node_num_cpu:sum))
            ||| % $._config,
          },
          {
            // CPU saturation is 1min avg run queue length / number of CPUs.
            // Can go over 100%.  >100% is bad.
            record: ':node_cpu_saturation_load1:',
            expr: |||
              sum(node_load1{%(nodeExporterSelector)s})
              /
              sum(node:node_num_cpu:sum)
            ||| % $._config,
          },
          {
            // CPU saturation is 1min avg run queue length / number of CPUs.
            // Can go over 100%.  >100% is bad.
            record: 'node:node_cpu_saturation_load1:',
            expr: |||
              sum by (node) (
                node_load1{%(nodeExporterSelector)s}
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
              /
              node:node_num_cpu:sum
            ||| % $._config,
          },
          {
            record: ':node_memory_utilisation:',
            expr: |||
              1 -
              sum(node_memory_MemFree_bytes{%(nodeExporterSelector)s} + node_memory_Cached_bytes{%(nodeExporterSelector)s} + node_memory_Buffers_bytes{%(nodeExporterSelector)s})
              /
              sum(node_memory_MemTotal_bytes{%(nodeExporterSelector)s})
            ||| % $._config,
          },
          // Add separate rules for Free & Total, so we can aggregate across clusters
          // in dashboards.
          {
            record: ':node_memory_MemFreeCachedBuffers_bytes:sum',
            expr: |||
              sum(node_memory_MemFree_bytes{%(nodeExporterSelector)s} + node_memory_Cached_bytes{%(nodeExporterSelector)s} + node_memory_Buffers_bytes{%(nodeExporterSelector)s})
            ||| % $._config,
          },
          {
            record: ':node_memory_MemTotal_bytes:sum',
            expr: |||
              sum(node_memory_MemTotal_bytes{%(nodeExporterSelector)s})
            ||| % $._config,
          },
          {
            // Available memory per node
            // SINCE 2018-02-08
            record: 'node:node_memory_bytes_available:sum',
            expr: |||
              sum by (node) (
                (node_memory_MemFree_bytes{%(nodeExporterSelector)s} + node_memory_Cached_bytes{%(nodeExporterSelector)s} + node_memory_Buffers_bytes{%(nodeExporterSelector)s})
                * on (namespace, %(podLabel)s) group_left(node)
                  node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            // Total memory per node
            // SINCE 2018-02-08
            record: 'node:node_memory_bytes_total:sum',
            expr: |||
              sum by (node) (
                node_memory_MemTotal_bytes{%(nodeExporterSelector)s}
                * on (namespace, %(podLabel)s) group_left(node)
                  node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            // Memory utilisation per node, normalized by per-node memory
            // NEW 2018-02-08
            record: 'node:node_memory_utilisation:ratio',
            expr: |||
              (node:node_memory_bytes_total:sum - node:node_memory_bytes_available:sum)
              /
              node:node_memory_bytes_total:sum
            |||,
          },
          {
            // Memory utilisation per node, normalized by cluster-wide memory
            record: 'node:cluster_memory_utilisation:ratio',
            expr: |||
              (node:node_memory_bytes_total:sum - node:node_memory_bytes_available:sum)
              /
              scalar(sum(node:node_memory_bytes_total:sum))
            |||,
          },
          {
            record: ':node_memory_swap_io_bytes:sum_rate',
            expr: |||
              1e3 * sum(
                (rate(node_vmstat_pgpgin{%(nodeExporterSelector)s}[1m])
               + rate(node_vmstat_pgpgout{%(nodeExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
          {
            // DEPRECATED
            record: 'node:node_memory_utilisation:',
            expr: |||
              1 -
              sum by (node) (
                (node_memory_MemFree_bytes{%(nodeExporterSelector)s} + node_memory_Cached_bytes{%(nodeExporterSelector)s} + node_memory_Buffers_bytes{%(nodeExporterSelector)s})
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
              /
              sum by (node) (
                node_memory_MemTotal_bytes{%(nodeExporterSelector)s}
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            // DEPENDS 2018-02-08
            // REPLACE node:node_memory_utilisation:
            record: 'node:node_memory_utilisation_2:',
            expr: |||
              1 - (node:node_memory_bytes_available:sum / node:node_memory_bytes_total:sum)
            ||| % $._config,
          },
          {
            record: 'node:node_memory_swap_io_bytes:sum_rate',
            expr: |||
              1e3 * sum by (node) (
                (rate(node_vmstat_pgpgin{%(nodeExporterSelector)s}[1m])
               + rate(node_vmstat_pgpgout{%(nodeExporterSelector)s}[1m]))
               * on (namespace, %(podLabel)s) group_left(node)
                 node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            // Disk utilisation (seconds spent, by rate() it's bound by 1 second)
            record: ':node_disk_utilisation:avg_irate',
            expr: |||
              avg(irate(node_disk_io_time_seconds_total{%(nodeExporterSelector)s,%(diskDeviceSelector)s}[1m]))
            ||| % $._config,
          },
          {
            // Disk utilisation (seconds spent, by rate() it's bound by 1 second)
            record: 'node:node_disk_utilisation:avg_irate',
            expr: |||
              avg by (node) (
                irate(node_disk_io_time_seconds_total{%(nodeExporterSelector)s,%(diskDeviceSelector)s}[1m])
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            // Disk saturation (seconds spent, by rate() it's bound by 1 second)
            record: ':node_disk_saturation:avg_irate',
            expr: |||
              avg(irate(node_disk_io_time_weighted_seconds_total{%(nodeExporterSelector)s,%(diskDeviceSelector)s}[1m]))
            ||| % $._config,
          },
          {
            // Disk saturation (seconds spent, by rate() it's bound by 1 second)
            record: 'node:node_disk_saturation:avg_irate',
            expr: |||
              avg by (node) (
                irate(node_disk_io_time_weighted_seconds_total{%(nodeExporterSelector)s,%(diskDeviceSelector)s}[1m])
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            record: 'node:node_filesystem_usage:',
            expr: |||
              max by (instance, namespace, %(podLabel)s, device) ((node_filesystem_size_bytes{%(fstypeSelector)s}
              - node_filesystem_avail_bytes{%(fstypeSelector)s})
              / node_filesystem_size_bytes{%(fstypeSelector)s})
            ||| % $._config,
          },
          {
            record: 'node:node_filesystem_avail:',
            expr: |||
              max by (instance, namespace, %(podLabel)s, device) (node_filesystem_avail_bytes{%(fstypeSelector)s} / node_filesystem_size_bytes{%(fstypeSelector)s})
            ||| % $._config,
          },
          {
            record: ':node_net_utilisation:sum_irate',
            expr: |||
              sum(irate(node_network_receive_bytes_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m])) +
              sum(irate(node_network_transmit_bytes_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:node_net_utilisation:sum_irate',
            expr: |||
              sum by (node) (
                (irate(node_network_receive_bytes_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]) +
                irate(node_network_transmit_bytes_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]))
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            record: ':node_net_saturation:sum_irate',
            expr: |||
              sum(irate(node_network_receive_drop_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m])) +
              sum(irate(node_network_transmit_drop_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:node_net_saturation:sum_irate',
            expr: |||
              sum by (node) (
                (irate(node_network_receive_drop_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]) +
                irate(node_network_transmit_drop_total{%(nodeExporterSelector)s,%(hostNetworkInterfaceSelector)s}[1m]))
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              )
            ||| % $._config,
          },
          {
            record: 'node:node_inodes_total:',
            expr: |||
              max(
                max(
                  kube_pod_info{%(kubeStateMetricsSelector)s, host_ip!=""}
                ) by (node, host_ip)
                * on (host_ip) group_right (node)
                label_replace(
                  (max(node_filesystem_files{%(nodeExporterSelector)s, %(hostMountpointSelector)s}) by (instance)), "host_ip", "$1", "instance", "(.*):.*"
                )
              ) by (node)
            ||| % $._config,
          },
          {
            record: 'node:node_inodes_free:',
            expr: |||
              max(
                max(
                  kube_pod_info{%(kubeStateMetricsSelector)s, host_ip!=""}
                ) by (node, host_ip)
                * on (host_ip) group_right (node)
                label_replace(
                  (max(node_filesystem_files_free{%(nodeExporterSelector)s, %(hostMountpointSelector)s}) by (instance)), "host_ip", "$1", "instance", "(.*):.*"
                )
              ) by (node)
            ||| % $._config,
          },
        ],
      },
    ],
  },
}
