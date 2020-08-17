local config =  import 'config.libsonnet';
local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;
local template = grafana.template;
local singlestat = grafana.singlestat;

local legendConfig = {
        legend+: {
            sideWidth: 250,
        },
    };

local dashboardConfig = {
        uid: config._config.grafanaDashboardIDs['ghproxy.json'],
    };

local histogramQuantileTarget(phi) = prometheus.target(
        std.format('histogram_quantile(%s, sum(rate(github_request_duration_bucket{job="ghproxy", token_hash="${token}", path="${path}", status="${status}"}[5m])) by (le))', phi),
        legendFormat=std.format('phi=%s', phi),
    );

local histogramQuantileTargetOverview(phi) = prometheus.target(
        std.format('histogram_quantile(%s, sum(rate(github_request_duration_bucket{job="ghproxy"}[5m])) by (le))', phi),
        legendFormat=std.format('phi=%s', phi),
    );

local mytemplate(name, labelInQuery) = template.new(
        name,
        'prometheus',
        std.format('label_values(github_request_duration_count{job="ghproxy"}, %s)', labelInQuery),
        label=name,
        refresh='time',
    );

dashboard.new(
        'GitHub Cache',
        time_from='now-7d',
        schemaVersion=18,
        refresh='1m',
      )
.addTemplate(mytemplate('token', 'token_hash'))
.addTemplate(mytemplate('path', 'path'))
.addTemplate(mytemplate('status', 'status'))
.addTemplate(
  {
        "allValue": null,
        "current": {
          "text": "30m",
          "value": "30m"
        },
        "hide": 0,
        "includeAll": false,
        "label": "range",
        "multi": false,
        "name": "range",
        "options":
        [
          {
            "selected": false,
            "text": '%s' % r,
            "value": '%s'% r,
          },
          for r in ['24h', '12h', '6h', '3h', '1h']
        ] +
        [
          {
            "selected": true,
            "text": '30m',
            "value": '30m',
          }
        ] +
        [
          {
            "selected": false,
            "text": '%s' % r,
            "value": '%s'% r,
          },
          for r in ['30m', '15m', '10m', '5m']
        ],
        "query": "3h,1h,30m,15m,10m,5m",
        "skipUrlSync": false,
        "type": "custom"
      }
)
.addPanel(
    (graphPanel.new(
        'Cache Requests (per hour)',
        description='Count of cache requests of each cache mode over the last hour.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_avg=true,
        legend_sort='avg',
        legend_sortDesc=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(increase(ghcache_responses[1h])) by (mode)',
        legendFormat='{{mode}}',
    ))
    .addTarget(prometheus.target(
        'sum(increase(ghcache_responses{mode=~"COALESCED|REVALIDATED"}[1h]))',
        legendFormat='(No Cost)',
    )), gridPos={
    h: 6,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'Cache Efficiency',
        description='Percentage of cacheable requests that are fulfilled for free.\nNo cost modes are "COALESCED" and "REVALIDATED".\nCacheable modes include the no cost modes, "CHANGED" and "MISS".',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        min='0',
        max='1',
        formatY1='percentunit',
        #TODO: uncomment When this merges, https://github.com/grafana/grafonnet-lib/pull/122
        #y_axis_label='% Cacheable Request Fulfilled for Free',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(increase(ghcache_responses{mode=~"COALESCED|REVALIDATED"}[1h])) \n/ sum(increase(ghcache_responses{mode=~"COALESCED|REVALIDATED|MISS|CHANGED"}[1h]))',
        legendFormat='Efficiency',
    )), gridPos={
    h: 6,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'Disk Usage',
        description='',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        stack=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'avg(ghcache_disk_used) without (instance,pod)',
        legendFormat='GB Used',
    ))
    .addTarget(prometheus.target(
        'avg(ghcache_disk_free) without (instance,pod)',
        legendFormat='GB Free',
    )), gridPos={
    h: 6,
    w: 16,
    x: 0,
    y: 0,
  })
.addPanel(
    singlestat.new(
        'API Tokens Saved: Last hour',
        description='The number of no cost requests in the last hour.\nThis includes both "COALESCED" and "REVALIDATED" modes.',
        datasource='prometheus',
        valueName='current',
    )
    .addTarget(prometheus.target(
        'sum(increase(ghcache_responses{mode=~"COALESCED|REVALIDATED"}[1h]))',
        instant=true,
    )), gridPos={
    h: 6,
    w: 4,
    x: 16,
    y: 0,
  })
.addPanel(
    singlestat.new(
        'API Tokens Saved: Last 7 days',
        description='The number of no cost requests in the last 7 days.\nThis includes both "COALESCED" and "REVALIDATED" modes.',
        datasource='prometheus',
        valueName='current',
        format='short',
    )
    .addTarget(prometheus.target(
        'sum(increase(ghcache_responses{mode=~"COALESCED|REVALIDATED"}[7d]))',
        instant=true,
    )), gridPos={
    h: 6,
    w: 4,
    x: 20,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'Token Usage',
        description='GitHub token usage by token identifier and API version.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        min='0',
        max='5000',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'label_replace(sum(github_token_usage) by (api_version, token_hash), "token_hash_short", "$1", "token_hash", "([a-z0-9]{5})(.*)")',
         legendFormat='{{api_version}}:{{token_hash_short}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'Request Rates: Overview by status with ${range}',
        description='GitHub request rates by status.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(rate(github_request_duration_count{job="ghproxy"}[${range}])) by (status)',
         legendFormat='{{status}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'Request Rates: Overview by path for ${status} with ${range}',
        description='GitHub request rates by path.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(rate(github_request_duration_count{status="${status}",job="ghproxy"}[${range}])) by (path)',
         legendFormat='{{path}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'Request Rates: ${token}, ${path}, and ${status} with ${range}',
        description='GitHub request rates by token identifier, path and status.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_sort='current',
        legend_sortDesc=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'label_replace(sum(rate(github_request_duration_count{job="ghproxy", token_hash="${token}", path="${path}", status="${status}"}[${range}])) by (token_hash, path, status), "token_hash_short", "$1", "token_hash", "([a-z0-9]{5})(.*)")',
         legendFormat='{{status}}:{{token_hash_short}}:{{path}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'Latency Distribution Overview with ${range}',
        description='histogram_quantile(<phi>, sum(rate(github_request_duration_bucket{job="ghproxy"}[${range}])) by (le))',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_sort='avg',
        legend_sortDesc=true,
    ) + legendConfig)
    .addTarget(histogramQuantileTargetOverview('0.99'))
    .addTarget(histogramQuantileTargetOverview('0.95'))
    .addTarget(histogramQuantileTargetOverview('0.5')), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'Latency Distribution for ${token}, ${path}, and ${status} with ${range}',
        description='histogram_quantile(<phi>, sum(rate(github_request_duration_bucket{job="ghproxy", token_hash=~"${token}", path=~"${path}", status=~"${status}"}[${range}])) by (le))',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_sort='avg',
        legend_sortDesc=true,
    ) + legendConfig)
    .addTarget(histogramQuantileTarget('0.99'))
    .addTarget(histogramQuantileTarget('0.95'))
    .addTarget(histogramQuantileTarget('0.5')), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    (graphPanel.new(
        'GitHub Request Timeout Rates: Overview by path with ${range}',
        description='GitHub request timeout rates by path.',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(rate(github_request_timeouts_bucket[${range}])) by (path)',
         legendFormat='{{path}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
+ dashboardConfig
