local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;


local dashboardConfig = {
        uid: '950e4d81ca8c2272d9717cc35ce80381',
    };

dashboard.new(
        'sinker dashboard',
        time_from='now-1h',
        schemaVersion=18,
      )
.addPanel(
    graphPanel.new(
        'time used in each sinker cleaning',
        description='sum(sinker_loop_duration_seconds)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_loop_duration_seconds)'
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })
.addPanel(
    graphPanel.new(
        'existing pods/prow jobs',
        description='sum(sinker_pods_existing) and sum(sinker_prow_jobs_existing)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_pods_existing)')
    )
    .addTarget(prometheus.target(
        'sum(sinker_prow_jobs_existing)')
    ), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 18,
  })
.addPanel(
    graphPanel.new(
        'removed pods',
        description='sum(sinker_pods_removed) by (reason)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_pods_removed) by (reason)'
    )), gridPos={
    h: 9,
    w: 12,
    x: 0,
    y: 27,
  })
.addPanel(
    graphPanel.new(
        'cleaned prow jobs',
        description='sum(sinker_prow_jobs_cleaned) by (reason)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_prow_jobs_cleaned) by (reason)'

    )), gridPos={
    h: 9,
    w: 12,
    x: 12,
    y: 27,
  })
.addPanel(
        graphPanel.new(
        'errors occurred in pod cleaning',
        description='sum(sinker_pod_removal_errors) by (reason)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_pod_removal_errors) by (reason)'
    )), gridPos={
    h: 9,
    w: 12,
    x: 0,
    y: 27,
  })
  .addPanel(
    graphPanel.new(
        'errors occurred in prow job cleaning',
        description='sum(sinker_prow_jobs_cleaning_errors) by (reason)',
        datasource='prometheus',
    )
    .addTarget(prometheus.target(
        'sum(sinker_prow_jobs_cleaning_errors) by (reason)'
    )), gridPos={
    h: 9,
    w: 12,
    x: 12,
    y: 27,
  })
+ dashboardConfig
