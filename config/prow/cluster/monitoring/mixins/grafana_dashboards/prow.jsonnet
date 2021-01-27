local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;
local prometheus = grafana.prometheus;

local legendConfig = {
        legend+: {
            sideWidth: 250
        },
    };

local dashboardConfig = {
        uid: '970b051d3adfd62eb592154c5ce80377',
    };

dashboard.new(
        'prow dashboard',
        time_from='now-1d',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'up',
        description='sum by(job) (up)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum by(job) (up)',
        legendFormat='{{job}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })
+ dashboardConfig
