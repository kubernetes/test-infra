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
        'tide dashboard',
        time_from='now-2d',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'Tide Pool Sizes',
        description="The number of PRs eligible for merge in each Tide pool.",
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'avg(pooledprs and ((time() - updatetime) < 240)) by (org, repo, branch)',
        legendFormat='{{org}}/{{repo}}:{{branch}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    (graphPanel.new(
        'Tide Daily Merge Rate',
        description="Calculated on a 24 hour interval.",
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_sort='avg',
        legend_sortDesc=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        '(sum(rate(merges_sum[1d]) > 0) by (org, repo, branch)) * 86400',
        legendFormat='{{org}}/{{repo}}:{{branch}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
.addPanel(
    (graphPanel.new(
        'Tide Daily Merge Rate: Batches Only',
        description="Calculated on a 24 hour interval.",
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_alignAsTable=true,
        legend_rightSide=true,
        legend_sort='avg',
        legend_sortDesc=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        '(sum(    rate(merges_sum[1d]) - (sum(rate(merges_bucket{le=\"1\"}[1d])) without (le)) > 0     ) by (org, repo, branch)) * 86400',
        legendFormat='{{org}}/{{repo}}:{{branch}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 27,
  })
.addPanel(
    //TODO: Merge Event + Recent merges: might be related the protmetheus setting
    (graphPanel.new(
        'Tide Pool: kubernetes/kubernetes:master',
        description="Tide stats for the master branch of the kubernetes/kubernetes repo.\nSpecifically, the number of pooled PRs and the daily merge rate.\n(See the more general graphs for details on how these are calculated.)",
        datasource='prometheus',
        legend_values=true,
        legend_current=true,
        legend_avg=true,
        legend_alignAsTable=true,
        legend_rightSide=true,
        nullPointMode='null as zero',
    ) + legendConfig)
    .addTarget(prometheus.target(
        'avg(pooledprs{org=\"kubernetes\",repo=\"kubernetes\",branch=\"master\"} and ((time() - updatetime) < 240)) or vector(0)',
        legendFormat='Pool size',
    )).addTarget(prometheus.target(
        'sum(rate(merges_sum{org=\"kubernetes\",repo=\"kubernetes\",branch=\"master\"}[1d])) * 86400',
        legendFormat='Daily merge rate',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 36,
  })
.addPanel(
    (graphPanel.new(
        'Tide Processing Time (seconds)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'max(syncdur and (changes(syncdur[1h]) > 0))',
        legendFormat='Sync time',
    )).addTarget(prometheus.target(
        'max(statusupdatedur and (changes(statusupdatedur[1h]) > 0))',
        legendFormat='Status update time',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 45,
  })
+ dashboardConfig
