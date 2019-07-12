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
        uid: '970b051d3adfd62eb592154c5ce80377',
    };

dashboard.new(
        'prow dashboard',
        time_from='now-1d',
        schemaVersion=18,
      )
.addPanel(
    (graphPanel.new(
        'up',
        description='sum by(job) (up)',
        datasource='prometheus',
        legend_alignAsTable=true,
        legend_rightSide=true,
    ) + legendConfig)
    .addTarget(prometheus.target(
        'sum by(job) (up)',
        legendFormat='{{job}}',
    )), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  })

.addPanel(
  (graphPanel.new(
     'CPU',
     description='CPU usage',
     datasource='prometheus-k8s',
     legend_alignAsTable=true,
     legend_rightSide=true,
   ) + legendConfig)
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="hook"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='hook',
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="tide"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tide'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="deck"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="deck-internal"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck-internal'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="plank"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='plank'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="sinker"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='sinker'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="statusreconciler"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='statusreconciler'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="ghproxy"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='ghproxy'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="horologium"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='horologium'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="tot"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tot'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="needs-rebase"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='needs-rebase'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="kata-jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='kata-jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="jenkins-dev-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-dev-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="artifact-uploader"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='artifact-uploader'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="refresh"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='refresh'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_cpu_usage:sum{namespace="ci"} * on (pod_name) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="cherrypick"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='cherrypick'
    )
  ), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  }
)

.addPanel(
  (graphPanel.new(
     'Memory',
     description='Memory usage',
     datasource='prometheus-k8s',
     legend_alignAsTable=true,
     legend_rightSide=true,
     formatY1='decbytes',
   ) + legendConfig)
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="hook"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='hook'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="tide"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tide'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="deck"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="deck-internal"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck-internal'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="plank"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='plank'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="sinker"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='sinker'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="statusreconciler"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='statusreconciler'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="ghproxy"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='ghproxy'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="horologium"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='horologium'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="tot"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tot'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="needs-rebase"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='needs-rebase'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="kata-jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='kata-jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="jenkins-dev-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-dev-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="artifact-uploader"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='artifact-uploader'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="refresh"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='refresh'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(container_memory_working_set_bytes{namespace="ci",container_name!="POD"} * on (pod_name) group_left(pod) label_replace(kube_pod_labels{pod!="",label_app="prow", label_component="cherrypick"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='cherrypick'
    )
  ), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  }
)

.addPanel(
  (graphPanel.new(
     'Ephemeral storage',
     description='Ephemeral storage',
     datasource='prometheus-k8s',
     legend_alignAsTable=true,
     legend_rightSide=true,
     formatY1='decbytes',
   ) + legendConfig)
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="hook"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='hook',
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="tide"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tide'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="deck"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="deck-internal"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='deck-internal'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="plank"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='plank'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="sinker"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='sinker'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="statusreconciler"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='statusreconciler'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="ghproxy"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='ghproxy'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="horologium"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='horologium'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="tot"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='tot'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="needs-rebase"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='needs-rebase'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="kata-jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='kata-jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="jenkins-dev-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-dev-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="jenkins-operator"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='jenkins-operator'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="artifact-uploader"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='artifact-uploader'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="refresh"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='refresh'
    )
  )
  .addTarget(
    prometheus.target(
      'sum(pod_name:container_fs_usage_bytes:sum{namespace="ci"} * on (pod_name, namespace) label_replace(kube_pod_labels{pod!="",label_app="prow",label_component="cherrypick"}, "pod_name", "$1", "pod", "(.*)"))',
      legendFormat='cherrypick'
    )
  ), gridPos={
    h: 9,
    w: 24,
    x: 0,
    y: 0,
  }
)
+ dashboardConfig
