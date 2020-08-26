local g = import 'grafana-builder/grafana.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local template = grafana.template;

{
  grafanaDashboards+::
    if $._config.showMultiCluster then {
      'k8s-resources-multicluster.json':
        local tableStyles = {
          [$._config.clusterLabel]: {
            alias: 'Cluster',
            link: '%(prefix)s/d/%(uid)s/k8s-resources-cluster?var-datasource=$datasource&var-cluster=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-cluster.json') },
          },
        };

        g.dashboard(
          '%(dashboardNamePrefix)sCompute Resources /  Multi-Cluster' % $._config.grafanaK8s,
          uid=($._config.grafanaDashboardIDs['k8s-resources-multicluster.json']),
        ).addRow(
          (g.row('Headlines') +
           {
             height: '100px',
             showTitle: false,
           })
          .addPanel(
            g.panel('CPU Utilisation') +
            g.statPanel('1 - avg(rate(node_cpu_seconds_total{mode="idle"}[$__interval]))' % $._config)
          )
          .addPanel(
            g.panel('CPU Requests Commitment') +
            g.statPanel('sum(kube_pod_container_resource_requests_cpu_cores) / sum(kube_node_status_allocatable_cpu_cores)' % $._config)
          )
          .addPanel(
            g.panel('CPU Limits Commitment') +
            g.statPanel('sum(kube_pod_container_resource_limits_cpu_cores) / sum(kube_node_status_allocatable_cpu_cores)' % $._config)
          )
          .addPanel(
            g.panel('Memory Utilisation') +
            g.statPanel('1 - sum(:node_memory_MemAvailable_bytes:sum) / sum(kube_node_status_allocatable_memory_bytes)' % $._config)
          )
          .addPanel(
            g.panel('Memory Requests Commitment') +
            g.statPanel('sum(kube_pod_container_resource_requests_memory_bytes) / sum(kube_node_status_allocatable_memory_bytes)' % $._config)
          )
          .addPanel(
            g.panel('Memory Limits Commitment') +
            g.statPanel('sum(kube_pod_container_resource_limits_memory_bytes) / sum(kube_node_status_allocatable_memory_bytes)' % $._config)
          )
        )
        .addRow(
          g.row('CPU')
          .addPanel(
            g.panel('CPU Usage') +
            g.queryPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config)
            + { fill: 0, linewidth: 2 },
          )
        )
        .addRow(
          g.row('CPU Quota')
          .addPanel(
            g.panel('CPU Quota') +
            g.tablePanel([
              'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate) by (%(clusterLabel)s)' % $._config,
              'sum(kube_pod_container_resource_requests_cpu_cores) by (%(clusterLabel)s)' % $._config,
              'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate) by (%(clusterLabel)s) / sum(kube_pod_container_resource_requests_cpu_cores) by (%(clusterLabel)s)' % $._config,
              'sum(kube_pod_container_resource_limits_cpu_cores) by (%(clusterLabel)s)' % $._config,
              'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate) by (%(clusterLabel)s) / sum(kube_pod_container_resource_limits_cpu_cores) by (%(clusterLabel)s)' % $._config,
            ], tableStyles {
              'Value #A': { alias: 'CPU Usage' },
              'Value #B': { alias: 'CPU Requests' },
              'Value #C': { alias: 'CPU Requests %', unit: 'percentunit' },
              'Value #D': { alias: 'CPU Limits' },
              'Value #E': { alias: 'CPU Limits %', unit: 'percentunit' },
            })
          )
        )
        .addRow(
          g.row('Memory')
          .addPanel(
            g.panel('Memory Usage (w/o cache)') +
            // Not using container_memory_usage_bytes here because that includes page cache
            g.queryPanel('sum(container_memory_rss{container!=""}) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config) +
            { fill: 0, linewidth: 2, yaxes: g.yaxes('bytes') },
          )
        )
        .addRow(
          g.row('Memory Requests')
          .addPanel(
            g.panel('Requests by Namespace') +
            g.tablePanel([
              // Not using container_memory_usage_bytes here because that includes page cache
              'sum(container_memory_rss{container!=""}) by (%(clusterLabel)s)' % $._config,
              'sum(kube_pod_container_resource_requests_memory_bytes) by (%(clusterLabel)s)' % $._config,
              'sum(container_memory_rss{container!=""}) by (%(clusterLabel)s) / sum(kube_pod_container_resource_requests_memory_bytes) by (%(clusterLabel)s)' % $._config,
              'sum(kube_pod_container_resource_limits_memory_bytes) by (%(clusterLabel)s)' % $._config,
              'sum(container_memory_rss{container!=""}) by (%(clusterLabel)s) / sum(kube_pod_container_resource_limits_memory_bytes) by (%(clusterLabel)s)' % $._config,
            ], tableStyles {
              'Value #A': { alias: 'Memory Usage', unit: 'bytes' },
              'Value #B': { alias: 'Memory Requests', unit: 'bytes' },
              'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
              'Value #D': { alias: 'Memory Limits', unit: 'bytes' },
              'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
            })
          )
        ) + { tags: $._config.grafanaK8s.dashboardTags, refresh: $._config.grafanaK8s.refresh },
    } else {},
}
