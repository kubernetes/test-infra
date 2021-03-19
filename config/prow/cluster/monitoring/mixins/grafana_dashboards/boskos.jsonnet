local grafana = import 'grafonnet/grafana.libsonnet';
local config =  import 'config.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;


local dashboardConfig = {
        uid: 'wSrfvNxWz',
    };

dashboard.new(
        'Boskos Resource Usage',
        time_from='now-1d',
        schemaVersion=18,
      )
.addPanels([
    graphPanel.new(
        '%s usage status' % resource.friendly,
        description='The number of %ss in each state.' % resource.friendly,
        datasource='prometheus',
        stack=true,
        fill=3,
        linewidth=0,
        aliasColors={'busy': '#ff0000', 'cleaning': '#00eeff', 'dirty': '#ff8000', 'free': '#00ff00', 'leased': '#ee00ff', 'other': '#aaaaff', 'toBeDeleted': '#fafa00', 'tombstone': '#cccccc'}
    )
    .addTarget(prometheus.target(
        std.format('sum(boskos_resources{type="%s",job="%s"} or boskos_resources{type="%s",instance="%s"}) by (state)', [resource.type, resource.job, resource.type, resource.instance]),
        legendFormat='{{state}}',
    ))
    {gridPos: {
        h: 9,
        w: 24,
        x: 0,
        y: 0,
    }}
      for resource in config._config.boskosResourcetypes
  ])
+ dashboardConfig
