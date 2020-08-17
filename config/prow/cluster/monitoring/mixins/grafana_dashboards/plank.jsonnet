local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;

local legendConfig = {
        legend+: {
            sideWidth: 250
        },
    };

local dashboardConfig = {
        uid: 'e1778910572e3552a935c2035ce80369',
    };

dashboard.new(
        'plank dashboard',
        time_from='now-1h',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'number of Prow jobs by type',
        description='sum(prowjobs{job="plank"}) by (type)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(prowjobs{job="plank"}) by (type)',
        legendFormat='{{type}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'number of Prow jobs by state',
        description='sum(prowjobs{job="plank"}) by (state)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(prowjobs{job="plank"}) by (state)',
        legendFormat='{{state}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
+ dashboardConfig
