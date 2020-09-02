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

    local podTemplate =
      template.new(
        name='pod',
        datasource='$datasource',
        query='label_values(kube_pod_info{%(clusterLabel)s="$cluster", namespace="$namespace"}, pod)' % $._config.clusterLabel,
        current='',
        hide='',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    'k8s-resources-pod.json':
      local tableStyles = {
        container: {
          alias: 'Container',
        },
      };

      local cpuRequestsQuery = |||
        sum(
            kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"})
      ||| % $._config;

      local cpuLimitsQuery = std.strReplace(cpuRequestsQuery, 'requests', 'limits');
      local memRequestsQuery = std.strReplace(cpuRequestsQuery, 'cpu_cores', 'memory_bytes');
      local memLimitsQuery = std.strReplace(cpuLimitsQuery, 'cpu_cores', 'memory_bytes');

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Pod' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-pod.json']),
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel(
            [
              'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{namespace="$namespace", pod="$pod", container!="POD", %(clusterLabel)s="$cluster"}) by (container)' % $._config,
              cpuRequestsQuery,
              cpuLimitsQuery,
            ], [
              '{{container}}',
              'requests',
              'limits',
            ],
          ) +
          g.stack + {
            seriesOverrides: [
              {
                alias: 'requests',
                color: '#F2495C',
                fill: 0,
                hideTooltip: true,
                legend: true,
                linewidth: 2,
                stack: false,
              },
              {
                alias: 'limits',
                color: '#FF9830',
                fill: 0,
                hideTooltip: true,
                legend: true,
                linewidth: 2,
                stack: false,
              },
            ],
          },
        )
      )
      .addRow(
        g.row('CPU Throttling')
        .addPanel(
          g.panel('CPU Throttling') +
          g.queryPanel('sum(increase(container_cpu_cfs_throttled_periods_total{namespace="$namespace", pod="$pod", container!="POD", container!="", %(clusterLabel)s="$cluster"}[5m])) by (container) /sum(increase(container_cpu_cfs_periods_total{namespace="$namespace", pod="$pod", container!="POD", container!="", %(clusterLabel)s="$cluster"}[5m])) by (container)' % $._config, '{{container}}') +
          g.stack
          + {
            yaxes: g.yaxes({ format: 'percentunit', max: 1 }),
            legend+: {
              current: true,
              max: true,
            },
            thresholds: [
              {
                value: $._config.cpuThrottlingPercent / 100,
                colorMode: 'critical',
                op: 'gt',
                fill: true,
                line: true,
                yaxis: 'left',
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
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container!="POD"}) by (container)' % $._config,
            'sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
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
          g.panel('Memory Usage') +
          g.queryPanel([
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container!="POD", container!="", image!=""}) by (container)' % $._config,
            memRequestsQuery,
            memLimitsQuery,
          ], [
            '{{container}}',
            'requests',
            'limits',
          ]) +
          g.stack +
          {
            yaxes: g.yaxes('bytes'),
            seriesOverrides: [
              {
                alias: 'requests',
                color: '#F2495C',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
              {
                alias: 'limits',
                color: '#FF9830',
                dashes: true,
                fill: 0,
                hideTooltip: true,
                legend: false,
                linewidth: 2,
                stack: false,
              },
            ],
          }
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container!="POD", container!="", image!=""}) by (container)' % $._config,
            'sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", image!=""}) by (container) / sum(kube_pod_container_resource_requests_memory_bytes{namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container!=""}) by (container)' % $._config,
            'sum(container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container!="", image!=""}) by (container) / sum(kube_pod_container_resource_limits_memory_bytes{namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(container_memory_rss{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container != "", container != "POD"}) by (container)' % $._config,
            'sum(container_memory_cache{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container != "", container != "POD"}) by (container)' % $._config,
            'sum(container_memory_swap{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container != "", container != "POD"}) by (container)' % $._config,
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
          g.panel('Receive Bandwidth') +
          g.queryPanel('sum(irate(container_network_receive_bytes_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Transmit Bandwidth') +
          g.queryPanel('sum(irate(container_network_transmit_bytes_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets') +
          g.queryPanel('sum(irate(container_network_transmit_packets_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Received Packets Dropped') +
          g.queryPanel('sum(irate(container_network_receive_packets_dropped_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Rate of Transmitted Packets Dropped') +
          g.queryPanel('sum(irate(container_network_transmit_packets_dropped_total{namespace=~"$namespace", pod=~"$pod"}[$__interval])) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps'), interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      ) + { tags: $._config.grafanaK8s.dashboardTags, templating+: { list+: [clusterTemplate, namespaceTemplate, podTemplate] }, refresh: $._config.grafanaK8s.refresh },
  },
}
