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

    local nodeTemplate =
      template.new(
        name='node',
        datasource='$datasource',
        query='label_values(kube_pod_info{%(clusterLabel)s="$cluster"}, node)' % $._config.clusterLabel,
        current='',
        hide='',
        refresh=1,
        includeAll=false,
        multi=true,
        sort=1
      ),

    'k8s-resources-node.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
        },
      };

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Node (Pods)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-node.json']),
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config, '{{pod}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod) / sum(kube_pod_container_resource_requests_cpu_cores{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod) / sum(kube_pod_container_resource_limits_cpu_cores{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
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
          g.queryPanel('sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node", container!=""}) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('bytes') },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_requests_memory_bytes{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod) / sum(kube_pod_container_resource_requests_memory_bytes{node=~"$node"}) by (pod)' % $._config,
            'sum(kube_pod_container_resource_limits_memory_bytes{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod) / sum(kube_pod_container_resource_limits_memory_bytes{node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_rss{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_cache{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_swap{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
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
      ) + { tags: $._config.grafanaK8s.dashboardTags, refresh: $._config.grafanaK8s.refresh, templating+: { list+: [clusterTemplate, nodeTemplate] } },
  },
}
