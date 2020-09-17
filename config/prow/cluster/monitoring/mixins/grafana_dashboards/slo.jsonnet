local config =  import 'config.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;


local dashboardConfig = {
        uid: config._config.grafanaDashboardIDs['slo.json'],
    };

dashboard.new(
        'SLO Compliance Dashboard',
        time_from='now-7d',
        schemaVersion=18,
      )
.addPanel(
    graphPanel.new(
        'Prow overall SLO compliance',
        description='slo_prow_ok',
        datasource='prometheus',
        legend_rightSide=true,
    )
    .addTarget(prometheus.target(
        'slo_prow_ok'
    )),
    gridPos={h: 4, w: 24, x: 0, y: 0})
.addPanels([
    graphPanel.new(
        '%s SLO compliance' % comp,
        description='slo_component_ok{slo="%s"}' % comp,
        datasource='prometheus',
        legend_rightSide=true,
    )
    .addTarget(prometheus.target(
        'slo_component_ok{slo="%s"}' % comp
    ))

    for comp in config._config.slo.components
])
+ dashboardConfig
