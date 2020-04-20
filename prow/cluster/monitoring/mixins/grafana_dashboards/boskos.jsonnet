local grafana = import 'grafonnet/grafana.libsonnet';
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
        'sum(boskos_resources{type="%s"}) by (state)' % resource.type,
        legendFormat='{{state}}',
    ))
    {gridPos: {
        h: 9,
        w: 24,
        x: 0,
        y: 0,
    }}
      for resource in [
        {type: "aws-account", friendly: "AWS account"},
        {type: "gce-project", friendly: "GCE project"},
        {type: "gke-project", friendly: "GKE project"},
        {type: "gpu-project", friendly: "GPU project"},
        {type: "ingress-project", friendly: "Ingress project"},
        {type: "istio-project", friendly: "Istio project"},
        {type: "node-e2e-project", friendly: "Node e2e project"},
        {type: "scalability-project", friendly: "Scalability project"},
        {type: "scalability-presubmit-project", friendly: "Scalability presubmit project"}
      ]
  ])
+ dashboardConfig
