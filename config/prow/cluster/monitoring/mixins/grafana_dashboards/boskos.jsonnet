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
        std.format('sum(boskos_resources{type="%s",instance="%s"}) by (state)', [resource.type, resource.instance]),
        legendFormat='{{state}}',
    ))
    {gridPos: {
        h: 9,
        w: 24,
        x: 0,
        y: 0,
    }}
      for resource in [
        {instance: "104.197.27.114:9090", type: "aws-account", friendly: "AWS account"},
        {instance: "104.197.27.114:9090", type: "gce-project", friendly: "GCE project"},
        {instance: "35.225.208.117:9090", type: "gce-project", friendly: "GCE project (k8s-infra)"},
        {instance: "104.197.27.114:9090", type: "gke-project", friendly: "GKE project"},
        {instance: "104.197.27.114:9090", type: "gpu-project", friendly: "GPU project"},
        {instance: "35.225.208.117:9090", type: "gpu-project", friendly: "GPU project (k8s-infra)"},
        {instance: "104.197.27.114:9090", type: "ingress-project", friendly: "Ingress project"},
        {instance: "104.197.27.114:9090", type: "node-e2e-project", friendly: "Node e2e project"},
        {instance: "104.197.27.114:9090", type: "scalability-project", friendly: "Scalability project"},
        {instance: "35.225.208.117:9090", type: "scalability-project", friendly: "Scalability project (k8s-infra)"},
        {instance: "104.197.27.114:9090", type: "scalability-presubmit-project", friendly: "Scalability presubmit project"}
      ]
  ])
+ dashboardConfig
