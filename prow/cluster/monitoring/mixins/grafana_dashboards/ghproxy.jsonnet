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
        uid: 'd72fe8d0400b2912e319b1e95d0ab1b3',
    };

dashboard.new(
        'GitHub Cache',
        time_from='now-7d',
        schemaVersion=18,
        refresh='1m',
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
        min='0',
        max='1',
        formatY1='percentunit',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'avg(ghcache_disk_used/(ghcache_disk_used+ghcache_disk_free)) without(instance)',
        legendFormat='% Used',
    ))
    .addTarget(prometheus.target(
        'avg(ghcache_disk_used) without(instance)',
        legendFormat='GB Used',
    ))
    .addTarget(prometheus.target(
        'avg(ghcache_disk_free) without(instance)',
        legendFormat='GB Free',
    ))
    .addSeriesOverride({
              alias: 'GB Used',
              lines: false,
              yaxis: 2,
            })
    .addSeriesOverride({
              alias: 'GB Free',
              lines: false,
              yaxis: 2,
            })
    .resetYaxes()
    .addYaxis(
      format='percentunit',
      min=0,
      max=1,
      label=null,
      show=true,
    )
    .addYaxis(
      format='decgbytes',
      min=0,
      max=105,
      label=null,
      show=false,
    ), gridPos={
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
+ dashboardConfig
