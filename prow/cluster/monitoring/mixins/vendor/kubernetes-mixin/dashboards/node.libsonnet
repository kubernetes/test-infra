local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local promgrafonnet = import '../lib/promgrafonnet/promgrafonnet.libsonnet';
local numbersinglestat = promgrafonnet.numbersinglestat;
local gauge = promgrafonnet.gauge;

{
  grafanaDashboards+:: {
    'nodes.json':
      local systemLoad =
        graphPanel.new(
          'System load',
          datasource='$datasource',
          span=6,
          format='short',
        )
        .addTarget(prometheus.target('max(node_load1{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='load 1m'))
        .addTarget(prometheus.target('max(node_load5{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='load 5m'))
        .addTarget(prometheus.target('max(node_load15{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='load 15m'))
        .addTarget(prometheus.target('count(node_cpu_seconds_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance", mode="user"})' % $._config, legendFormat='logical cores'));

      local cpuByCore =
        graphPanel.new(
          'Usage Per Core',
          datasource='$datasource',
          span=6,
          format='percentunit',
        )
        .addTarget(prometheus.target('sum by (cpu) (irate(node_cpu_seconds_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, mode!="idle", instance="$instance"}[5m]))' % $._config, legendFormat='{{cpu}}'));

      local memoryGraph =
        graphPanel.new(
          'Memory Usage',
          datasource='$datasource',
          span=9,
          format='bytes',
        )
        .addTarget(prometheus.target(
          |||
            max(
              node_memory_MemTotal_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_MemFree_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_Buffers_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_Cached_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
            )
          ||| % $._config, legendFormat='memory used'
        ))
        .addTarget(prometheus.target('max(node_memory_Buffers_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory buffers'))
        .addTarget(prometheus.target('max(node_memory_Cached_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory cached'))
        .addTarget(prometheus.target('max(node_memory_MemFree_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory free'));

      local memoryGauge = gauge.new(
        'Memory Usage',
        |||
          max(
            (
              (
                node_memory_MemTotal_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_MemFree_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_Buffers_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_memory_Cached_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              )
              / node_memory_MemTotal_bytes{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
            ) * 100)
        ||| % $._config,
      ).withLowerBeingBetter();

      // cpu
      local cpuGraph = graphPanel.new(
        'CPU Utilization',
        datasource='$datasource',
        span=9,
        format='percent',
        max=100,
        min=0,
        legend_show='true',
        legend_values='true',
        legend_min='false',
        legend_max='false',
        legend_current='true',
        legend_total='false',
        legend_avg='true',
        legend_alignAsTable='true',
        legend_rightSide='true',
      ).addTarget(prometheus.target(
        |||
          max (sum by (cpu) (irate(node_cpu_seconds_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, mode!="idle", instance="$instance"}[2m])) ) * 100
        ||| % $._config,
        legendFormat='{{ cpu }}',
        intervalFactor=10,
      ));

      local cpuGauge = gauge.new(
        'CPU Usage',
        |||
          avg(sum by (cpu) (irate(node_cpu_seconds_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, mode!="idle", instance="$instance"}[2m]))) * 100
        ||| % $._config,
      ).withLowerBeingBetter();

      local diskIO =
        graphPanel.new(
          'Disk I/O',
          datasource='$datasource',
          span=6,
        )
        .addTarget(prometheus.target('max(rate(node_disk_read_bytes_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='read'))
        .addTarget(prometheus.target('max(rate(node_disk_written_bytes_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='written'))
        .addTarget(prometheus.target('max(rate(node_disk_io_time_seconds_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s,  instance="$instance"}[2m]))' % $._config, legendFormat='io time')) +
        {
          seriesOverrides: [
            {
              alias: 'read',
              yaxis: 1,
            },
            {
              alias: 'io time',
              yaxis: 2,
            },
          ],
          yaxes: [
            self.yaxe(format='bytes'),
            self.yaxe(format='ms'),
          ],
        };

      local diskSpaceUsage =
        graphPanel.new(
          'Disk Space Usage',
          datasource='$datasource',
          span=6,
          format='percentunit',
        )
        .addTarget(prometheus.target(
          'node:node_filesystem_usage:{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, legendFormat='disk used'
        ))
        .addTarget(prometheus.target(
          'node:node_filesystem_usage:{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, legendFormat='disk free'
        ));

      local networkReceived =
        graphPanel.new(
          'Network Received',
          datasource='$datasource',
          span=6,
          format='bytes',
          stack=true,
        )
        .addTarget(prometheus.target('rate(node_network_receive_bytes_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance", device!~"lo"}[5m])' % $._config, legendFormat='{{device}}'));

      local networkTransmitted =
        graphPanel.new(
          'Network Transmitted',
          datasource='$datasource',
          span=6,
          format='bytes',
          stack=true,
        )
        .addTarget(prometheus.target('rate(node_network_transmit_bytes_total{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance", device!~"lo"}[5m])' % $._config, legendFormat='{{device}}'));

      local inodesGraph =
        graphPanel.new(
          'Inodes Usage',
          datasource='$datasource',
          span=9,
        )
        .addTarget(prometheus.target(
          |||
            max(
              node_filesystem_files{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_filesystem_files_free{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
            )
          ||| % $._config, legendFormat='inodes used'
        ))
        .addTarget(prometheus.target('max(node_filesystem_files_free{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"})' % $._config, legendFormat='inodes free'));

      local inodesGauge = gauge.new(
        'Inodes Usage',
        |||
          max(
            (
              (
                node_filesystem_files{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              - node_filesystem_files_free{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
              )
              / node_filesystem_files{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s, instance="$instance"}
            ) * 100)
        ||| % $._config,
      ).withLowerBeingBetter();

      dashboard.new(
        '%(dashboardNamePrefix)sNodes' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['nodes.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'Prometheus',
            value: 'Prometheus',
          },
          hide: 0,
          label: null,
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: '',
          type: 'datasource',
        },
      )
      .addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(kube_pod_info, %s)' % $._config.clusterLabel,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
        )
      )
      .addTemplate(
        template.new(
          'instance',
          '$datasource',
          'label_values(node_boot_time_seconds{%(clusterLabel)s="$cluster", %(nodeExporterSelector)s}, instance)' % $._config,
          refresh='time',
        )
      )
      .addRow(
        row.new()
        .addPanel(systemLoad)
        .addPanel(cpuByCore)
      )
      .addRow(
        row.new()
        .addPanel(cpuGraph)
        .addPanel(cpuGauge)
      )
      .addRow(
        row.new()
        .addPanel(memoryGraph)
        .addPanel(memoryGauge)
      )
      .addRow(
        row.new()
        .addPanel(diskIO)
        .addPanel(diskSpaceUsage)
      )
      .addRow(
        row.new()
        .addPanel(networkReceived)
        .addPanel(networkTransmitted)
      )
      .addRow(
        row.new()
        .addPanel(inodesGraph)
        .addPanel(inodesGauge)
      ),
  },
}
