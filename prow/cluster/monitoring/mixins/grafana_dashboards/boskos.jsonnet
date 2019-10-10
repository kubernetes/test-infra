local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;

local legendConfig = {
        legend+: {
            sideWidth: 500
        },
    };

local dashboardConfig = {
        uid: 'd69a91f76d8110d3e72885ee5ce8038e',
    };

dashboard.new(
        'boskos dashboard',
        time_from='now-2d',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'Boskos GCE projects usage stats',
        description="",
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_gce_project_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_gce_project_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_gce_project_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_gce_project_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'Boskos GKE projects usage stats',
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_gke_project_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_gke_project_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_gke_project_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_gke_project_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
.addPanel(
    (graphPanel.new(
        'Boskos Ingress projects usage stats',
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_ingress_project_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_ingress_project_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_ingress_project_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_ingress_project_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 27,
  })
.addPanel(
    (graphPanel.new(
        'Boskos GPU projects usage stats',
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_gpu_project_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_gpu_project_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_gpu_project_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_gpu_project_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 36,
  })
.addPanel(
    (graphPanel.new(
        'Boskos Istio projects usage stats',
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_istio_project_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_istio_project_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_istio_project_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_istio_project_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 45,
  })
  .addPanel(
    (graphPanel.new(
        'Boskos AWS accounts usage stats',
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=false,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'boskos_aws_account_cleaning',
        format='time_series',
        intervalFactor=2,
        legendFormat='cleaning',
    ))
    .addTarget(prometheus.target(
        'boskos_aws_account_dirty',
        format='time_series',
        intervalFactor=2,
        legendFormat='dirty',
    ))
    .addTarget(prometheus.target(
        'boskos_aws_account_busy',
        format='time_series',
        intervalFactor=2,
        legendFormat='busy',
    ))
    .addTarget(prometheus.target(
        'boskos_aws_account_free',
        format='time_series',
        intervalFactor=2,
        legendFormat='free',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 54,
  })
+ dashboardConfig
