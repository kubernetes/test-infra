local g = import 'grafana-builder/grafana.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local template = grafana.template;

{
  grafanaDashboards+:: {
    local clusterTemplate =
      template.new(
        name='cluster',
        datasource='$datasource',
        query='label_values(node_cpu_seconds_total, %s)' % $._config.clusterLabel,
        current='',
        hide=if $._config.showMultiCluster then '' else '2',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    'k8s-resources-cluster.json':
      local tableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          linkTooltip: 'Drill down to pods',
        },
        'Value #A': {
          alias: 'Pods',
          linkTooltip: 'Drill down to pods',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell_1' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          decimals: 0,
        },
        'Value #B': {
          alias: 'Workloads',
          linkTooltip: 'Drill down to workloads',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-workloads-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell_1' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-workloads-namespace.json') },
          decimals: 0,
        },
      };


      local podWorkloadColumns = [
        'sum(kube_pod_owner{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
        'count(avg(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster"}) by (workload, namespace)) by (namespace)' % $._config,
      ];

      local networkColumns = [
        'sum(irate(container_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
        'sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
        'sum(irate(container_network_receive_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config,
      ];

      local networkTableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          linkTooltip: 'Drill down to pods',
        },
        'Value #A': {
          alias: 'Current Receive Bandwidth',
          unit: 'Bps',
        },
        'Value #B': {
          alias: 'Current Transmit Bandwidth',
          unit: 'Bps',
        },
        'Value #C': {
          alias: 'Rate of Received Packets',
          unit: 'pps',
        },
        'Value #D': {
          alias: 'Rate of Transmitted Packets',
          unit: 'pps',
        },
        'Value #E': {
          alias: 'Rate of Received Packets Dropped',
          unit: 'pps',
        },
        'Value #F': {
          alias: 'Rate of Transmitted Packets Dropped',
          unit: 'pps',
        },
      };

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Cluster' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-cluster.json']),
      )
      .addRow(
        (g.row('Headlines') +
         {
           height: '100px',
           showTitle: false,
         })
        .addPanel(
          g.panel('CPU Utilisation') +
          g.statPanel('1 - avg(rate(node_cpu_seconds_total{mode="idle", %(clusterLabel)s="$cluster"}[$__interval]))' % $._config) +
          { interval: $._config.grafanaK8s.minimumTimeInterval },
        )
        .addPanel(
          g.panel('CPU Requests Commitment') +
          g.statPanel('sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable_cpu_cores{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('CPU Limits Commitment') +
          g.statPanel('sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable_cpu_cores{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Utilisation') +
          g.statPanel('1 - sum(:node_memory_MemAvailable_bytes:sum{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable_memory_bytes{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Requests Commitment') +
          g.statPanel('sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable_memory_bytes{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Limits Commitment') +
          g.statPanel('sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable_memory_bytes{%(clusterLabel)s="$cluster"})' % $._config)
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel(podWorkloadColumns + [
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
          ], tableStyles {
            'Value #C': { alias: 'CPU Usage' },
            'Value #D': { alias: 'CPU Requests' },
            'Value #E': { alias: 'CPU Requests %', unit: 'percentunit' },
            'Value #F': { alias: 'CPU Limits' },
            'Value #G': { alias: 'CPU Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Usage (w/o cache)') +
          // Not using container_memory_usage_bytes here because that includes page cache
          g.queryPanel('sum(container_memory_rss{%(clusterLabel)s="$cluster", container!=""}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('bytes') },
        )
      )
      .addRow(
        g.row('Memory Requests')
        .addPanel(
          g.panel('Requests by Namespace') +
          g.tablePanel(podWorkloadColumns + [
            // Not using container_memory_usage_bytes here because that includes page cache
            'sum(container_memory_rss{%(clusterLabel)s="$cluster", container!=""}) by (namespace)' % $._config,
            'sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(container_memory_rss{%(clusterLabel)s="$cluster", container!=""}) by (namespace) / sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(container_memory_rss{%(clusterLabel)s="$cluster", container!=""}) by (namespace) / sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
          ], tableStyles {
            'Value #C': { alias: 'Memory Usage', unit: 'bytes' },
            'Value #D': { alias: 'Memory Requests', unit: 'bytes' },
            'Value #E': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #F': { alias: 'Memory Limits', unit: 'bytes' },
            'Value #G': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Current Network Usage') +
          g.tablePanel(
            networkColumns,
            networkTableStyles
          ) +
          { interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Receive Bandwidth') +
          g.queryPanel('sum(irate(container_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Transmit Bandwidth') +
          g.queryPanel('sum(irate(container_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Average Container Bandwidth by Namespace: Received') +
          g.queryPanel('avg(irate(container_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Average Container Bandwidth by Namespace: Transmitted') +
          g.queryPanel('avg(irate(container_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets Dropped') +
          g.queryPanel('sum(irate(container_network_receive_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets Dropped') +
          g.queryPanel('sum(irate(container_network_transmit_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[$__interval])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      ) + {
        tags: $._config.grafanaK8s.dashboardTags,
        templating+: { list+: [clusterTemplate] },
        refresh: $._config.grafanaK8s.refresh,
      },
  },
}
