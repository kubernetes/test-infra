local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;

{
  grafanaDashboards+:: {
    'controller-manager.json':
      local upCount =
        singlestat.new(
          'Up',
          datasource='$datasource',
          span=2,
          valueName='min',
        )
        .addTarget(prometheus.target('sum(up{%(kubeControllerManagerSelector)s})' % $._config));

      local workQueueAddRate =
        graphPanel.new(
          'Work Queue Add Rate',
          datasource='$datasource',
          span=10,
          format='ops',
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('sum(rate(workqueue_adds_total{%(kubeControllerManagerSelector)s, instance=~"$instance"}[5m])) by (instance, name)' % $._config, legendFormat='{{instance}} {{name}}'));

      local workQueueDepth =
        graphPanel.new(
          'Work Queue Depth',
          datasource='$datasource',
          span=12,
          min=0,
          format='short',
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('sum(rate(workqueue_depth{%(kubeControllerManagerSelector)s, instance=~"$instance"}[5m])) by (instance, name)' % $._config, legendFormat='{{instance}} {{name}}'));

      local workQueueLatency =
        graphPanel.new(
          'Work Queue Latency',
          datasource='$datasource',
          span=12,
          format='s',
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(workqueue_queue_duration_seconds_bucket{%(kubeControllerManagerSelector)s, instance=~"$instance"}[5m])) by (instance, name, le))' % $._config, legendFormat='{{instance}} {{name}}'));

      local rpcRate =
        graphPanel.new(
          'Kube API Request Rate',
          datasource='$datasource',
          span=4,
          format='ops',
        )
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeControllerManagerSelector)s, instance=~"$instance",code=~"2.."}[5m]))' % $._config, legendFormat='2xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeControllerManagerSelector)s, instance=~"$instance",code=~"3.."}[5m]))' % $._config, legendFormat='3xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeControllerManagerSelector)s, instance=~"$instance",code=~"4.."}[5m]))' % $._config, legendFormat='4xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeControllerManagerSelector)s, instance=~"$instance",code=~"5.."}[5m]))' % $._config, legendFormat='5xx'));

      local postRequestLatency =
        graphPanel.new(
          'Post Request Latency 99th Quantile',
          datasource='$datasource',
          span=8,
          format='s',
          min=0,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_latency_seconds_bucket{%(kubeControllerManagerSelector)s, instance=~"$instance", verb="POST"}[5m])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local getRequestLatency =
        graphPanel.new(
          'Get Request Latency 99th Quantile',
          datasource='$datasource',
          span=12,
          format='s',
          min=0,
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_latency_seconds_bucket{%(kubeControllerManagerSelector)s, instance=~"$instance", verb="GET"}[5m])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          span=4,
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(kubeControllerManagerSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          span=4,
          format='short',
          min=0,
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(kubeControllerManagerSelector)s,instance=~"$instance"}[5m])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          span=4,
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(kubeControllerManagerSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));


      dashboard.new(
        '%(dashboardNamePrefix)sController Manager' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['controller-manager.json']),
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
          'label_values(process_cpu_seconds_total{%(kubeControllerManagerSelector)s}, instance)' % $._config,
          refresh='time',
          includeAll=true,
        )
      )
      .addRow(
        row.new()
        .addPanel(upCount)
        .addPanel(workQueueAddRate)
      ).addRow(
        row.new()
        .addPanel(workQueueDepth)
      ).addRow(
        row.new()
        .addPanel(workQueueLatency)
      ).addRow(
        row.new()
        .addPanel(rpcRate)
        .addPanel(postRequestLatency)
      ).addRow(
        row.new()
        .addPanel(getRequestLatency)
      ).addRow(
        row.new()
        .addPanel(memory)
        .addPanel(cpu)
        .addPanel(goroutines)
      ),
  },
}
