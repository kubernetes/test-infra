local grafana = import 'grafonnet/grafana.libsonnet';
local annotation = grafana.annotation;
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;
local promgrafonnet = import '../lib/promgrafonnet/promgrafonnet.libsonnet';
local row = grafana.row;
local singlestat = grafana.singlestat;
local template = grafana.template;
local numbersinglestat = promgrafonnet.numbersinglestat;

{
  grafanaDashboards+:: {
    'pods.json':
      local memoryRow = row.new()
                        .addPanel(
        graphPanel.new(
          'Memory Usage',
          datasource='$datasource',
          min=0,
          span=12,
          format='bytes',
          legend_rightSide=true,
          legend_alignAsTable=true,
          legend_current=true,
          legend_avg=true,
        )
        .addTarget(prometheus.target(
          'sum by(container) (container_memory_usage_bytes{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container=~"$container", container!="POD"})' % $._config,
          legendFormat='Current: {{ container }}',
        ))
        .addTarget(prometheus.target(
          'sum by(container) (kube_pod_container_resource_requests{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", resource="memory", pod="$pod", container=~"$container"})' % $._config,
          legendFormat='Requested: {{ container }}',
        ))
        .addTarget(prometheus.target(
          'sum by(container) (kube_pod_container_resource_limits{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", resource="memory", pod="$pod", container=~"$container"})' % $._config,
          legendFormat='Limit: {{ container }}',
        ))
        .addTarget(prometheus.target(
          'sum by(container) (container_memory_cache{%(cadvisorSelector)s, namespace="$namespace", pod=~"$pod", container=~"$container", container!="POD"})' % $._config,
          legendFormat='Cache: {{ container }}',
        ))
      );

      local cpuRow = row.new()
                     .addPanel(
        graphPanel.new(
          'CPU Usage',
          datasource='$datasource',
          min=0,
          span=12,
          legend_rightSide=true,
          legend_alignAsTable=true,
          legend_current=true,
          legend_avg=true,
        )
        .addTarget(prometheus.target(
          'sum by (container) (irate(container_cpu_usage_seconds_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", image!="", pod="$pod", container=~"$container", container!="POD"}[4m]))' % $._config,
          legendFormat='Current: {{ container }}',
        ))
        .addTarget(prometheus.target(
          'sum by(container) (kube_pod_container_resource_requests{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", resource="cpu", pod="$pod", container=~"$container"})' % $._config,
          legendFormat='Requested: {{ container }}',
        ))
        .addTarget(prometheus.target(
          'sum by(container) (kube_pod_container_resource_limits{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", resource="cpu", pod="$pod", container=~"$container"})' % $._config,
          legendFormat='Limit: {{ container }}',
        ))
      );

      local networkRow = row.new()
                         .addPanel(
        graphPanel.new(
          'Network I/O',
          datasource='$datasource',
          format='bytes',
          min=0,
          span=12,
          legend_rightSide=true,
          legend_alignAsTable=true,
          legend_current=true,
          legend_avg=true,
        )
        .addTarget(prometheus.target(
          'sort_desc(sum by (pod) (irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}[4m])))' % $._config,
          legendFormat='RX: {{ pod }}',
        ))
        .addTarget(prometheus.target(
          'sort_desc(sum by (pod) (irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}[4m])))' % $._config,
          legendFormat='TX: {{ pod }}',
        ))
      );

      local restartsRow = row.new()
                          .addPanel(
        graphPanel.new(
          'Total Restarts Per Container',
          datasource='$datasource',
          format='short',
          min=0,
          span=12,
          legend_rightSide=true,
          legend_alignAsTable=true,
          legend_current=true,
          legend_avg=true,
        )
        .addTarget(prometheus.target(
          'max by (container) (kube_pod_container_status_restarts_total{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod", container=~"$container"})' % $._config,
          legendFormat='Restarts: {{ container }}',
        ))
      );

      local restartAnnotation = annotation.datasource(
        'Restarts',
        '$datasource',
        expr='time() == BOOL timestamp(rate(kube_pod_container_status_restarts_total{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}[2m]) > 0)' % $._config,
        enable=true,
        hide=false,
        iconColor='rgba(215, 44, 44, 1)',
        tags=['restart'],
        type='rows',
        builtIn=1,
      );

      dashboard.new(
        '%(dashboardNamePrefix)sPods' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['pods.json']),
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
          'label_values(kube_pod_info, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
        )
      )
      .addTemplate(
        template.new(
          'namespace',
          '$datasource',
          'label_values(kube_pod_info{%(clusterLabel)s="$cluster"}, namespace)' % $._config,
          label='Namespace',
          refresh='time',
        )
      )
      .addTemplate(
        template.new(
          'pod',
          '$datasource',
          'label_values(kube_pod_info{%(clusterLabel)s="$cluster", namespace=~"$namespace"}, pod)' % $._config,
          label='Pod',
          refresh='time',
        )
      )
      .addTemplate(
        template.new(
          'container',
          '$datasource',
          'label_values(kube_pod_container_info{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}, container)' % $._config,
          label='Container',
          refresh='time',
          includeAll=true,
        )
      )
      .addAnnotation(restartAnnotation)
      .addRow(memoryRow)
      .addRow(cpuRow)
      .addRow(networkRow)
      .addRow(restartsRow),
  },
}
