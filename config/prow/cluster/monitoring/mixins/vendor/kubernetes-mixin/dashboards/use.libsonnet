local g = import 'grafana-builder/grafana.libsonnet';

{
  grafanaDashboards+:: {
    'k8s-cluster-rsrc-use.json':
      local legendLink = '%(prefix)s/d/%(uid)s/k8s-node-rsrc-use' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-node-rsrc-use.json') };

      g.dashboard(
        '%(dashboardNamePrefix)sUSE Method / Cluster' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-cluster-rsrc-use.json']),
      ).addTemplate('cluster', 'kube_node_info', $._config.clusterLabel, hide=if $._config.showMultiCluster then 0 else 2)
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:cluster_cpu_utilisation:ratio{%(clusterLabel)s="$cluster"}' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('CPU Saturation (Load1)') +
          g.queryPanel('node:node_cpu_saturation_load1:{%(clusterLabel)s="$cluster"} / scalar(sum(min(kube_pod_info{%(clusterLabel)s="$cluster"}) by (node)))' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation') +
          g.queryPanel('node:cluster_memory_utilisation:ratio{%(clusterLabel)s="$cluster"}' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O)') +
          g.queryPanel('node:node_memory_swap_io_bytes:sum_rate{%(clusterLabel)s="$cluster"}' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          // Full utilisation would be all disks on each node spending an average of
          // 1 sec per second doing I/O, normalize by node count for stacked charts
          g.queryPanel('node:node_disk_utilisation:avg_irate{%(clusterLabel)s="$cluster"} / scalar(:kube_pod_info_node_count:{%(clusterLabel)s="$cluster"})' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Disk IO Saturation') +
          g.queryPanel('node:node_disk_saturation:avg_irate{%(clusterLabel)s="$cluster"} / scalar(:kube_pod_info_node_count:{%(clusterLabel)s="$cluster"})' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:node_net_utilisation:sum_irate{%(clusterLabel)s="$cluster"}' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:node_net_saturation:sum_irate{%(clusterLabel)s="$cluster"}' % $._config, '{{node}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Storage')
        .addPanel(
          g.panel('Disk Capacity') +
          g.queryPanel(
            |||
              sum(max(node_filesystem_size_bytes{%(fstypeSelector)s, %(clusterLabel)s="$cluster"} - node_filesystem_avail_bytes{%(fstypeSelector)s, %(clusterLabel)s="$cluster"}) by (device,%(podLabel)s,namespace)) by (%(podLabel)s,namespace)
              / scalar(sum(max(node_filesystem_size_bytes{%(fstypeSelector)s, %(clusterLabel)s="$cluster"}) by (device,%(podLabel)s,namespace)))
              * on (namespace, %(podLabel)s) group_left (node) node_namespace_pod:kube_pod_info:{%(clusterLabel)s="$cluster"}
            ||| % $._config, '{{node}}', legendLink
          ) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        ),
      ) + { tags: $._config.grafanaK8s.dashboardTags },

    'k8s-node-rsrc-use.json':
      g.dashboard(
        '%(dashboardNamePrefix)sUSE Method / Node' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-node-rsrc-use.json']),
      ).addTemplate('cluster', 'kube_node_info', $._config.clusterLabel, hide=if $._config.showMultiCluster then 0 else 2)
      .addTemplate('node', 'kube_node_info{%(clusterLabel)s="$cluster"}' % $._config, 'node')
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:node_cpu_utilisation:avg1m{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          g.panel('CPU Saturation (Load1)') +
          g.queryPanel('node:node_cpu_saturation_load1:{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Saturation') +
          { yaxes: g.yaxes('percentunit') },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation') +
          g.queryPanel('node:node_memory_utilisation:{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Memory') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O)') +
          g.queryPanel('node:node_memory_swap_io_bytes:sum_rate{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Swap IO') +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          g.queryPanel('node:node_disk_utilisation:avg_irate{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          g.panel('Disk IO Saturation') +
          g.queryPanel('node:node_disk_saturation:avg_irate{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Saturation') +
          { yaxes: g.yaxes('percentunit') },
        )
      )
      .addRow(
        g.row('Net')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:node_net_utilisation:sum_irate{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:node_net_saturation:sum_irate{%(clusterLabel)s="$cluster", node="$node"}' % $._config, 'Saturation') +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk Utilisation') +
          g.queryPanel(
            |||
              node:node_filesystem_usage:{%(clusterLabel)s="$cluster"}
              * on (namespace, %(podLabel)s) group_left (node) node_namespace_pod:kube_pod_info:{%(clusterLabel)s="$cluster", node="$node"}
            ||| % $._config,
            '{{device}}',
          ) +
          { yaxes: g.yaxes('percentunit') },
        ),
      ) + { tags: $._config.grafanaK8s.dashboardTags },
  },
} + {
  grafanaDashboards+:: if $._config.showMultiCluster then {
    'k8s-multicluster-rsrc-use.json':
      local legendLink = '%(prefix)s/d/%(uid)s/k8s-cluster-rsrc-use' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-multicluster-rsrc-use.json') };

      g.dashboard(
        '%(dashboardNamePrefix)sUSE Method /  Multi-Cluster' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-cluster-rsrc-use.json']),
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('sum(node:node_cpu_utilisation:avg1m * node:node_num_cpu:sum) by (%(clusterLabel)s) / sum(node:node_num_cpu:sum) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('CPU Saturation (Load1)') +
          g.queryPanel('sum(node:node_cpu_saturation_load1:) by (%(clusterLabel)s) / sum(min(kube_pod_info) by (node, %(clusterLabel)s)) by(%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          // the metric `node:cluster_memory_utilisation:ratio` is each node's portion of the total cluster utilization; just sum them
          g.panel('Memory Utilisation') +
          g.queryPanel('sum(node:cluster_memory_utilisation:ratio) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O)') +
          g.queryPanel('sum(node:node_memory_swap_io_bytes:sum_rate) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          // Full utilisation would be all disks on each node spending an average of
          // 1 sec per second doing I/O, normalize by node count for stacked charts
          g.queryPanel('sum(node:node_disk_utilisation:avg_irate) by (%(clusterLabel)s) / sum(:kube_pod_info_node_count:) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Disk IO Saturation') +
          g.queryPanel('sum(node:node_disk_saturation:avg_irate) by (%(clusterLabel)s) / sum(:kube_pod_info_node_count:) by (%(clusterLabel)s) ' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('sum(node:node_net_utilisation:sum_irate) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('sum(node:node_net_saturation:sum_irate) by (%(clusterLabel)s)' % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Storage')
        .addPanel(
          g.panel('Disk Capacity') +
          g.queryPanel(
            |||
              sum(node_filesystem_size_bytes{%(fstypeSelector)s} - node_filesystem_avail_bytes{%(fstypeSelector)s}) by (%(clusterLabel)s)
              / sum(node_filesystem_size_bytes{%(fstypeSelector)s}) by (%(clusterLabel)s)
            ||| % $._config, '{{%(clusterLabel)s}}' % $._config, legendLink
          ) +
          { fill: 0, linewidth: 2, yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        ),
      ) + { tags: $._config.grafanaK8s.dashboardTags },
  } else {},
}
