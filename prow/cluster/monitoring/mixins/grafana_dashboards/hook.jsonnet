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
        uid: '6123f547a129441c2cdeac6c5ce802eb',
    };

dashboard.new(
        'hook dashboard',
        time_from='now-1h',
        schemaVersion=18,
      )
.addPanel(
    singlestat.new(
        'webhook counter',
        description='sum(prow_webhook_counter)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(prow_webhook_counter)',
        instant=true,
    )), gridPos={
    h: 4,
    w: 12,
    x: 0,
    y: 0,
  })
.addPanel(
    singlestat.new(
        'webhook response codes',
        description='sum(prow_webhook_response_codes)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(prow_webhook_response_codes)',
        instant=true,
    )), gridPos={
    h: 4,
    w: 12,
    x: 12,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'incoming webhooks by event type',
        description='sum(rate(prow_webhook_counter[1m])) by (event_type)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(rate(prow_webhook_counter[1m])) by (event_type)',
        legendFormat='{{event_type}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 4,
  })
.addPanel(
    (graphPanel.new(
        'webhook response codes',
        description='sum(rate(prow_webhook_response_codes[1m])) by (response_code)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(rate(prow_webhook_response_codes[1m])) by (response_code)',
        legendFormat='{{response_code}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 12,
    y: 13,
  })
.addPanel(
    (graphPanel.new(
        'configmap capacities',
        description='prow_configmap_size_bytes / 1048576',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_sort='current',
        legend_sortDesc=true,
        formatY1='percentunit',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'prow_configmap_size_bytes / 1048576',
        legendFormat='{{namespace}}/{{name}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 12,
    y: 13,
  })
+ dashboardConfig
