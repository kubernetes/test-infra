local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;

{
  grafanaDashboards+:: {
    'scheduler.json':
      local upCount =
        singlestat.new(
          'Up',
          datasource='$datasource',
          span=2,
          valueName='min',
        )
        .addTarget(prometheus.target('sum(up{%(kubeSchedulerSelector)s})' % $._config));

      local schedulingRate =
        graphPanel.new(
          'Scheduling Rate',
          datasource='$datasource',
          span=5,
          format='ops',
          min=0,
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('sum(rate(scheduler_e2e_scheduling_duration_seconds_count{%(kubeSchedulerSelector)s, instance=~"$instance"}[5m])) by (instance)' % $._config, legendFormat='{{instance}} e2e'))
        .addTarget(prometheus.target('sum(rate(scheduler_binding_duration_seconds_count{%(kubeSchedulerSelector)s, instance=~"$instance"}[5m])) by (instance)' % $._config, legendFormat='{{instance}} binding'))
        .addTarget(prometheus.target('sum(rate(scheduler_scheduling_algorithm_duration_seconds_count{%(kubeSchedulerSelector)s, instance=~"$instance"}[5m])) by (instance)' % $._config, legendFormat='{{instance}} scheduling algorithm'))
        .addTarget(prometheus.target('sum(rate(scheduler_volume_scheduling_duration_seconds_count{%(kubeSchedulerSelector)s, instance=~"$instance"}[5m])) by (instance)' % $._config, legendFormat='{{instance}} volume'));


      local schedulingLatency =
        graphPanel.new(
          'Scheduling latency 99th Quantile',
          datasource='$datasource',
          span=5,
          min=0,
          format='s',
          legend_show='true',
          legend_values='true',
          legend_current='true',
          legend_alignAsTable='true',
          legend_rightSide='true',
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_e2e_scheduling_duration_seconds_bucket{%(kubeSchedulerSelector)s,instance=~"$instance"}[5m])) by (instance, le))' % $._config, legendFormat='{{instance}} e2e'))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_binding_duration_seconds_bucket{%(kubeSchedulerSelector)s,instance=~"$instance"}[5m])) by (instance, le))' % $._config, legendFormat='{{instance}} binding'))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_scheduling_algorithm_duration_seconds_bucket{%(kubeSchedulerSelector)s,instance=~"$instance"}[5m])) by (instance, le))' % $._config, legendFormat='{{instance}} scheduling algorithm'))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_volume_scheduling_duration_seconds_bucket{%(kubeSchedulerSelector)s,instance=~"$instance"}[5m])) by (instance, le))' % $._config, legendFormat='{{instance}} volume'));

      local rpcRate =
        graphPanel.new(
          'Kube API Request Rate',
          datasource='$datasource',
          span=4,
          format='ops',
          min=0,
        )
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeSchedulerSelector)s, instance=~"$instance",code=~"2.."}[5m]))' % $._config, legendFormat='2xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeSchedulerSelector)s, instance=~"$instance",code=~"3.."}[5m]))' % $._config, legendFormat='3xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeSchedulerSelector)s, instance=~"$instance",code=~"4.."}[5m]))' % $._config, legendFormat='4xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(kubeSchedulerSelector)s, instance=~"$instance",code=~"5.."}[5m]))' % $._config, legendFormat='5xx'));

      local postRequestLatency =
        graphPanel.new(
          'Post Request Latency 99th Quantile',
          datasource='$datasource',
          span=8,
          format='s',
          min=0,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_latency_seconds_bucket{%(kubeSchedulerSelector)s, instance=~"$instance", verb="POST"}[5m])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

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
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_latency_seconds_bucket{%(kubeSchedulerSelector)s, instance=~"$instance", verb="GET"}[5m])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          span=4,
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(kubeSchedulerSelector)s, instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          span=4,
          format='bytes',
          min=0,
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(kubeSchedulerSelector)s, instance=~"$instance"}[5m])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          span=4,
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(kubeSchedulerSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));


      dashboard.new(
        '%(dashboardNamePrefix)sScheduler' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['scheduler.json']),
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
          'label_values(process_cpu_seconds_total{%(kubeSchedulerSelector)s}, instance)' % $._config,
          refresh='time',
          includeAll=true,
        )
      )
      .addRow(
        row.new()
        .addPanel(upCount)
        .addPanel(schedulingRate)
        .addPanel(schedulingLatency)
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
