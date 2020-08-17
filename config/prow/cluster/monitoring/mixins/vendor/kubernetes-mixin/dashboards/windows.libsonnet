local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local promgrafonnet = import '../lib/promgrafonnet/promgrafonnet.libsonnet';
local numbersinglestat = promgrafonnet.numbersinglestat;
local gauge = promgrafonnet.gauge;
local g = import 'grafana-builder/grafana.libsonnet';

{
  grafanaDashboards+:: {
    'k8s-resources-windows-cluster.json':
      local tableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-windows-namespace?var-datasource=$datasource&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-windows-namespace.json') },
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Cluster(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-cluster.json']),
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
      .addRow(
        (g.row('Headlines') +
         {
           height: '100px',
           showTitle: false,
         })
        .addPanel(
          g.panel('CPU Utilisation') +
          g.statPanel('1 - avg(rate(wmi_cpu_time_total{mode="idle"}[1m]))')
        )
        .addPanel(
          g.panel('CPU Requests Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_cpu_cores_request) / sum(node:windows_node_num_cpu:sum)')
        )
        .addPanel(
          g.panel('CPU Limits Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_cpu_cores_limit) / sum(node:windows_node_num_cpu:sum)')
        )
        .addPanel(
          g.panel('Memory Utilisation') +
          g.statPanel('1 - sum(:windows_node_memory_MemFreeCached_bytes:sum) / sum(:windows_node_memory_MemTotal_bytes:sum)')
        )
        .addPanel(
          g.panel('Memory Requests Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_memory_request) / sum(:windows_node_memory_MemTotal_bytes:sum)')
        )
        .addPanel(
          g.panel('Memory Limits Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_memory_limit) / sum(:windows_node_memory_MemTotal_bytes:sum)')
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate) by (namespace)', '{{namespace}}') +
          g.stack
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate) by (namespace)',
            'sum(kube_pod_windows_container_resource_cpu_cores_request) by (namespace)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate) by (namespace) / sum(kube_pod_windows_container_resource_cpu_cores_request) by (namespace)',
            'sum(kube_pod_windows_container_resource_cpu_cores_limit) by (namespace)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate) by (namespace) / sum(kube_pod_windows_container_resource_cpu_cores_limit) by (namespace)',
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
          g.panel('Memory Usage (Private Working Set)') +
          // Not using container_memory_usage_bytes here because that includes page cache
          g.queryPanel('sum(windows_container_private_working_set_usage{}) by (namespace)', '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('decbytes') },
        )
      )
      .addRow(
        g.row('Memory Requests')
        .addPanel(
          g.panel('Requests by Namespace') +
          g.tablePanel([
            // Not using container_memory_usage_bytes here because that includes page cache
            'sum(windows_container_private_working_set_usage{}) by (namespace)',
            'sum(kube_pod_windows_container_resource_memory_request) by (namespace)',
            'sum(windows_container_private_working_set_usage{}) by (namespace) / sum(kube_pod_windows_container_resource_memory_request) by (namespace)',
            'sum(kube_pod_windows_container_resource_memory_limit) by (namespace)',
            'sum(windows_container_private_working_set_usage{}) by (namespace) / sum(kube_pod_windows_container_resource_memory_limit) by (namespace)',
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      ),

    'k8s-resources-windows-namespace.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-windows-pod?var-datasource=$datasource&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-windows-pod.json') },
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Namespace(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-namespace.json']),
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
          'namespace',
          '$datasource',
          'label_values(windows_container_available, namespace)',
          label='Namespace',
          refresh='time',
        )
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace"}) by (pod)', '{{pod}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace"}) by (pod)',
            'sum(kube_pod_windows_container_resource_cpu_cores_request{namespace="$namespace"}) by (pod)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_cpu_cores_request{namespace="$namespace"}) by (pod)',
            'sum(kube_pod_windows_container_resource_cpu_cores_limit{namespace="$namespace"}) by (pod)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_cpu_cores_limit{namespace="$namespace"}) by (pod)',
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
          g.queryPanel('sum(windows_container_private_working_set_usage{namespace="$namespace"}) by (pod)', '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('decbytes') },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(windows_container_private_working_set_usage{namespace="$namespace"}) by (pod)',
            'sum(kube_pod_windows_container_resource_memory_request{namespace="$namespace"}) by (pod)',
            'sum(windows_container_private_working_set_usage{namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_memory_request{namespace="$namespace"}) by (pod)',
            'sum(kube_pod_windows_container_resource_memory_limit{namespace="$namespace"}) by (pod)',
            'sum(windows_container_private_working_set_usage{namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_memory_limit{namespace="$namespace"}) by (pod)',
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      ),

    'k8s-resources-windows-pod.json':
      local tableStyles = {
        container: {
          alias: 'Container',
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Pod(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-pod.json']),
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
          'namespace',
          '$datasource',
          'label_values(windows_container_available, namespace)',
          label='Namespace',
          refresh='time',
        )
      )
      .addTemplate(
        template.new(
          'pod',
          '$datasource',
          'label_values(windows_container_available{namespace="$namespace"}, pod)',
          label='Pod',
          refresh='time',
        )
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace", pod="$pod"}) by (container)', '{{container}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(kube_pod_windows_container_resource_cpu_cores_request{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_cpu_cores_request{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(kube_pod_windows_container_resource_cpu_cores_limit{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_cpu_cores_limit{namespace="$namespace", pod="$pod"}) by (container)',
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
          g.queryPanel('sum(windows_container_private_working_set_usage{namespace="$namespace", pod="$pod"}) by (container)', '{{container}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(windows_container_private_working_set_usage{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(kube_pod_windows_container_resource_memory_request{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(windows_container_private_working_set_usage{namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_memory_request{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(kube_pod_windows_container_resource_memory_limit{namespace="$namespace", pod="$pod"}) by (container)',
            'sum(windows_container_private_working_set_usage{namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_memory_limit{namespace="$namespace", pod="$pod"}) by (container)',
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Network I/O')
        .addPanel(
          graphPanel.new(
            'Network I/O',
            datasource='$datasource',
            format='bytes',
            min=0,
            legend_rightSide=true,
            legend_alignAsTable=true,
            legend_current=true,
            legend_avg=true,
          )
          .addTarget(prometheus.target(
            'sort_desc(sum by (container) (rate(windows_container_network_receive_bytes_total{namespace="$namespace", pod="$pod"}[1m])))' % $._config,
            legendFormat='Received : {{ container }}',
          ))
          .addTarget(prometheus.target(
            'sort_desc(sum by (container) (rate(windows_container_network_transmit_bytes_total{namespace="$namespace", pod="$pod"}[1m])))' % $._config,
            legendFormat='Transmitted : {{ container }}',
          ))
        )
      ),

    'k8s-windows-cluster-rsrc-use.json':
      local legendLink = '%(prefix)s/d/%(uid)s/k8s-windows-node-rsrc-use' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-windows-node-rsrc-use.json') };

      dashboard.new(
        '%(dashboardNamePrefix)sUSE Method / Cluster(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-windows-cluster-rsrc-use.json']),
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
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:windows_node_cpu_utilisation:avg1m * node:windows_node_num_cpu:sum / scalar(sum(node:windows_node_num_cpu:sum))', '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation') +
          g.queryPanel('node:windows_node_memory_utilisation:ratio', '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O Pages)') +
          g.queryPanel('node:windows_node_memory_swap_io_pages:irate', '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('short') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          // Full utilisation would be all disks on each node spending an average of
          // 1 sec per second doing I/O, normalize by node count for stacked charts
          g.queryPanel('node:windows_node_disk_utilisation:avg_irate / scalar(node:windows_node:sum)', '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:windows_node_net_utilisation:sum_irate', '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:windows_node_net_saturation:sum_irate', '{{instance}}', legendLink) +
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
              sum by (instance)(node:windows_node_filesystem_usage:)
            ||| % $._config, '{{instance}}', legendLink
          ) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        ),
      ),

    'k8s-windows-node-rsrc-use.json':
      dashboard.new(
        '%(dashboardNamePrefix)sUSE Method / Node(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-windows-node-rsrc-use.json']),
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
          'instance',
          '$datasource',
          'label_values(wmi_system_system_up_time, instance)',
          label='Instance',
          refresh='time',
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:windows_node_cpu_utilisation:avg1m{instance="$instance"}', 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          g.panel('CPU Usage Per Core') +
          g.queryPanel('sum by (core) (irate(wmi_cpu_time_total{%(wmiExporterSelector)s, mode!="idle", instance="$instance"}[5m]))' % $._config, '{{core}}') +
          { yaxes: g.yaxes('percentunit') },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation %') +
          g.queryPanel('node:windows_node_memory_utilisation:{instance="$instance"}', 'Memory') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          graphPanel.new('Memory Usage',
                         datasource='$datasource',
                         format='bytes',)
          .addTarget(prometheus.target(
            |||
              max(
                wmi_os_visible_memory_bytes{%(wmiExporterSelector)s, instance="$instance"}
                - wmi_memory_available_bytes{%(wmiExporterSelector)s, instance="$instance"}
              )
            ||| % $._config, legendFormat='memory used'
          ))
          .addTarget(prometheus.target('max(node:windows_node_memory_totalCached_bytes:sum{%(wmiExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory cached'))
          .addTarget(prometheus.target('max(wmi_memory_available_bytes{%(wmiExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory free'))
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O) Pages') +
          g.queryPanel('node:windows_node_memory_swap_io_pages:irate{instance="$instance"}', 'Swap IO') +
          { yaxes: g.yaxes('short') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          g.queryPanel('node:windows_node_disk_utilisation:avg_irate{instance="$instance"}', 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          graphPanel.new('Disk I/O',)
          .addTarget(prometheus.target('max(rate(wmi_logical_disk_read_bytes_total{%(wmiExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='read'))
          .addTarget(prometheus.target('max(rate(wmi_logical_disk_write_bytes_total{%(wmiExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='written'))
          .addTarget(prometheus.target('max(rate(wmi_logical_disk_read_seconds_total{%(wmiExporterSelector)s,  instance="$instance"}[2m]) + rate(wmi_logical_disk_write_seconds_total{%(wmiExporterSelector)s,  instance="$instance"}[2m]))' % $._config, legendFormat='io time')) +
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
          }
        )
      )
      .addRow(
        g.row('Net')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:windows_node_net_utilisation:sum_irate{instance="$instance"}', 'Utilisation') +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:windows_node_net_saturation:sum_irate{instance="$instance"}', 'Saturation') +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk Utilisation') +
          g.queryPanel(
            |||
              node:windows_node_filesystem_usage:{instance="$instance"}
            ||| % $._config,
            '{{volume}}',
          ) +
          { yaxes: g.yaxes('percentunit') },
        ),
      ),
  },
}
