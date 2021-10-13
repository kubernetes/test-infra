local config =  import 'config.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;
local template = grafana.template;

local legendConfig = {
        legend+: {
            sideWidth: 350
        },
    };

local dashboardConfig = {
        uid: config._config.grafanaDashboardIDs['boskos-http.json'],
    };

local histogramQuantileDuration(phi, selector='') = prometheus.target(
        std.format('histogram_quantile(%s, sum(rate(boskos_http_request_duration_seconds_bucket%s[5m])) by (le))', [phi, selector]),
        legendFormat=std.format('phi=%s', phi),
    );

local boskosTemplate(name, labelInQuery, includeAll) = template.new(
        name,
        'prometheus',
        std.format('label_values(boskos_http_request_duration_seconds_bucket, %s)', labelInQuery),
        label=name,
        allValues='.*',
        includeAll=includeAll,
        refresh='time',
    );

dashboard.new(
        'Boskos Server Dashboard',
        time_from='now-1h',
        schemaVersion=18,
      )
.addTemplate(boskosTemplate('instance', 'instance', false))
.addTemplate(boskosTemplate('path', 'path', true))
.addTemplate(boskosTemplate('status', 'status', true))
.addPanel(
    (graphPanel.new(
        'Latency distribution for instance ${instance} path ${path} and status ${status}',
        description='histogram_quantile(phi, sum(rate(boskos_http_request_duration_seconds_bucket[5m])) by (le))',
        datasource = 'prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_sort='avg',
        legend_sortDesc=true,
        ) + legendConfig)
        .addTarget(histogramQuantileDuration('0.99','{path=~"${path}", status=~"${status}", instance=~"${instance}"}'))
        .addTarget(histogramQuantileDuration('0.95','{path=~"${path}", status=~"${status}", instance=~"${instance}"}'))
        .addTarget(histogramQuantileDuration('0.5','{path=~"${path}", status=~"${status}", instance=~"${instance}"}')), gridPos={
        h: 9,
        w: 24,
        x: 0,
        y: 0
    })
.addPanel(
    (graphPanel.new(
        'Request rate for instance ${instance}',
        description='sum(rate(boskos_http_request_duration_seconds_count{instance=~"${instance}"}[5m])) by (path, status)',
        datasource = 'prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_sort='avg',
        legend_sortDesc=true,
        ) + legendConfig)
        .addTarget(prometheus.target(
            'sum(rate(boskos_http_request_duration_seconds_count{instance=~"${instance}"}[5m])) by (path, status)',
            legendFormat='{{path}} {{status}}'
        )), gridPos={
        h: 9,
        w: 24,
        x: 0,
        y: 0
    })
