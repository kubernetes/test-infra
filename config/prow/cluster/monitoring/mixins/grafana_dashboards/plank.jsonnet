local grafana = import 'grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local graphPanel = grafana.graphPanel;
local prometheus = grafana.prometheus;
local template = grafana.template;

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
.addTemplate(
  template.new(
    'cluster',
    'prometheus',
    'label_values(prowjobs{job=~"plank|prow-controller-manager"}, cluster)',
    label='cluster',
    allValues='.*',
    includeAll=true,
    refresh='time',
  )
)
.addTemplate(
  template.new(
    'org',
    'prometheus',
    'label_values(prowjobs{job=~"plank|prow-controller-manager"}, org)',
    label='org',
    allValues='.*',
    includeAll=true,
    refresh='time',
  )
)
.addTemplate(
  template.new(
    'repo',
    'prometheus',
    'label_values(prowjobs{job=~"plank|prow-controller-manager"}, repo)',
    label='repo',
    allValues='.*',
    includeAll=true,
    refresh='time',
  )
)
.addTemplate(
  template.custom(
    'state',
    'all,aborted,error,failure,pending,success,triggered',
    'all',
    label='state',
    includeAll=true,
    allValues='.*',
  )
)
.addTemplate(
  template.custom(
    'type',
    'all,batch,periodic,presubmit,postsubmit',
    'all',
    label='type',
    includeAll=true,
    allValues='.*',
  )
)
.addTemplate(
  template.custom(
    'group_by_1',
    'cluster,org,repo,state,type',
    'type',
    label='group_by_1',
    allValues='.*',
  )
)
.addTemplate(
  template.custom(
    'group_by_2',
    'cluster,org,repo,state,type',
    'state',
    label='group_by_2',
    allValues='.*',
  )
)
.addTemplate(
  template.custom(
    'group_by_3',
    'cluster,org,repo,state,type',
    'cluster',
    label='group_by_3',
    allValues='.*',
  )
)
.addPanel(
    (graphPanel.new(
        'number of Prow jobs (stacked) by ${group_by_1}',
        description='sum(prowjobs{...}) by (${group_by_1})',
        datasource='prometheus',
        stack=true,
        legend_alignAsTable=true,
        legend_rightSide=true,

    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(prowjobs{job=~"plank|prow-controller-manager",cluster=~"${cluster}",org=~"${org}",repo=~"${repo}",state=~"${state}",type=~"${type}"}) by (${group_by_1})',
        legendFormat='{{${group_by_1}}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
.addPanel(
    (graphPanel.new(
        'number of Prow jobs (stacked) by ${group_by_2}',
        description='sum(prowjobs{...}) by (${group_by_2})',
        datasource='prometheus',
        stack=true,
        legend_alignAsTable=true,
        legend_rightSide=true,

    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(prowjobs{job=~"plank|prow-controller-manager",cluster=~"${cluster}",org=~"${org}",repo=~"${repo}",state=~"${state}",type=~"${type}"}) by (${group_by_2})',
        legendFormat='{{${group_by_2}}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
.addPanel(
    (graphPanel.new(
        'number of Prow jobs (stacked) by ${group_by_3}',
        description='sum(prowjobs{...}) by (${group_by_3})',
        datasource='prometheus',
        stack=true,
        legend_alignAsTable=true,
        legend_rightSide=true,

    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum(prowjobs{job=~"plank|prow-controller-manager",cluster=~"${cluster}",org=~"${org}",repo=~"${repo}",state=~"${state}",type=~"${type}"}) by (${group_by_3})',
        legendFormat='{{${group_by_3}}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 9,
  })
+ dashboardConfig
