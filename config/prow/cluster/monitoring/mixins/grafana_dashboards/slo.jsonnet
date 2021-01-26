local config =  import 'config.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;


local legendConfig = {
        legend+: {
            sideWidth: 400
        },
    };

local dashboardConfig = {
        uid: config._config.grafanaDashboardIDs['slo.json'],
    };

dashboard.new(
        'SLO Compliance Dashboard',
        time_from='now-7d',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'Prow overall SLO compliance',
        description='slo_prow_ok',
        datasource='prometheus',
        legend_rightSide=true,
        decimals=0,
        min=0,
        max=1.25,
        labelY1='Compliant (T/F)',
        legend_values=true,
        legend_avg=true,
        legend_alignAsTable=true,
    )
    .addTarget(prometheus.target(
        'label_replace(slo_prow_ok, "__name__", "SLO", "", "")'
    )) + legendConfig),
    gridPos={h: 4, w: 24, x: 0, y: 0})
.addPanels([
    (graphPanel.new(
        '%s SLO compliance' % comp,
        description='slo_component_ok{slo="%s"}' % comp,
        datasource='prometheus',
        legend_rightSide=true,
        decimals=0,
        min=0,
        max=1.25,
        labelY1='Compliant (T/F)',
        legend_values=true,
        legend_avg=true,
        legend_alignAsTable=true,
    )
    .addTarget(prometheus.target(
        'label_replace(min(slo_component_ok{slo="%s"}) without (slo), "__name__", "SLO", "", "")' % comp
    )) + legendConfig)
    {gridPos:{h: 4, w: 24, x: 0, y: 0}}
    for comp in config._config.slo.components
])
+ dashboardConfig
