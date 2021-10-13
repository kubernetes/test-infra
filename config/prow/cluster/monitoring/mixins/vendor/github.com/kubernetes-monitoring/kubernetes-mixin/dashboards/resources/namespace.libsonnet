local g = import 'grafana-builder/grafana.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local template = grafana.template;

{
  grafanaDashboards+:: {
    local clusterTemplate =
      template.new(
        name='cluster',
        datasource='$datasource',
        query='label_values(kube_pod_info, %s)' % $._config.clusterLabel,
        current='',
        hide=if $._config.showMultiCluster then '' else '2',
        refresh=1,
        includeAll=false,
        sort=1
      ),

    local namespaceTemplate =
      template.new(
        name='namespace',
        datasource='$datasource',
        query='label_values(kube_pod_info{%(clusterLabel)s="$cluster"}, namespace)' % $._config.clusterLabel,
        current='',
        hide='',
        refresh=1,
        includeAll=false,
        sort=1
      ),
    'k8s-resources-namespace.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-pod?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-pod.json') },
        },
      };

      local networkColumns = [
        'sum(irate(container_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
        'sum(irate(container_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
        'sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
        'sum(irate(container_network_transmit_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
        'sum(irate(container_network_receive_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
        'sum(irate(container_network_transmit_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config,
      ];

      local networkTableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-pod?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-pod.json') },
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

      local cpuUsageQuery = 'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config;

      local memoryUsageQuery = 'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", container!="", image!=""}) by (pod)' % $._config;

      local cpuQuotaRequestsQuery = 'scalar(kube_resourcequota{%(clusterLabel)s="$cluster", namespace="$namespace", type="hard",resource="requests.cpu"})' % $._config;
      local cpuQuotaLimitsQuery = std.strReplace(cpuQuotaRequestsQuery, 'requests.cpu', 'limits.cpu');
      local memoryQuotaRequestsQuery = std.strReplace(cpuQuotaRequestsQuery, 'requests.cpu', 'requests.memory');
      local memoryQuotaLimitsQuery = std.strReplace(cpuQuotaRequestsQuery, 'requests.cpu', 'limits.memory');

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Namespace (Pods)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-namespace.json']),
      )
      .addRow(
        (g.row('Headlines') +
         {
           height: '100px',
           showTitle: false,
         })
        .addPanel(
          g.panel('CPU Utilisation (from requests)') +
          g.statPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) / sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"})' % $._config)
        )
        .addPanel(
          g.panel('CPU Utilisation (from limits)') +
          g.statPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) / sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Utilization (from requests)') +
          g.statPanel('sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace",container!="", image!=""}) / sum(kube_pod_container_resource_requests_memory_bytes{namespace="$namespace"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Utilisation (from limits)') +
          g.statPanel('sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace",container!="", image!=""}) / sum(kube_pod_container_resource_limits_memory_bytes{namespace="$namespace"})' % $._config)
        )
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel([
            cpuUsageQuery,
            cpuQuotaRequestsQuery,
            cpuQuotaLimitsQuery,
          ], ['{{pod}}', 'quota - requests', 'quota - limits']) +
          g.stack + {
            seriesOverrides: [
              {
                alias: 'quota - requests',
                color: '#F2495C',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
              {
                alias: 'quota - limits',
                color: '#FF9830',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
            ],
          },
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
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
        g.row('Memory Usage')
        .addPanel(
          g.panel('Memory Usage (w/o cache)') +
          // Like above, without page cache
          g.queryPanel([
            memoryUsageQuery,
            memoryQuotaRequestsQuery,
            memoryQuotaLimitsQuery,
          ], ['{{pod}}', 'quota - requests', 'quota - limits']) +
          g.stack +
          {
            yaxes: g.yaxes('bytes'),
            seriesOverrides: [
              {
                alias: 'quota - requests',
                color: '#F2495C',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
              {
                alias: 'quota - limits',
                color: '#FF9830',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
            ],
          },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace",container!="", image!=""}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace",container!="", image!=""}) by (pod) / sum(kube_pod_container_resource_requests_memory_bytes{namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace",container!="", image!=""}) by (pod) / sum(kube_pod_container_resource_limits_memory_bytes{namespace="$namespace"}) by (pod)' % $._config,
            'sum(container_memory_rss{%(clusterLabel)s="$cluster", namespace="$namespace",container!=""}) by (pod)' % $._config,
            'sum(container_memory_cache{%(clusterLabel)s="$cluster", namespace="$namespace",container!=""}) by (pod)' % $._config,
            'sum(container_memory_swap{%(clusterLabel)s="$cluster", namespace="$namespace",container!=""}) by (pod)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'bytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'bytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'bytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
            'Value #F': { alias: 'Memory Usage (RSS)', unit: 'bytes' },
            'Value #G': { alias: 'Memory Usage (Cache)', unit: 'bytes' },
            'Value #H': { alias: 'Memory Usage (Swap)', unit: 'bytes' },
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
          g.queryPanel('sum(irate(container_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Transmit Bandwidth') +
          g.queryPanel('sum(irate(container_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets Dropped') +
          g.queryPanel('sum(irate(container_network_receive_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets Dropped') +
          g.queryPanel('sum(irate(container_network_transmit_packets_dropped_total{%(clusterLabel)s="$cluster", %(namespaceLabel)s=~"$namespace"}[$__interval])) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      ) + { tags: $._config.grafanaK8s.dashboardTags, templating+: { list+: [clusterTemplate, namespaceTemplate] }, refresh: $._config.grafanaK8s.refresh },
  },
}
