local util = import 'config_util.libsonnet';

//
// Edit configuration in this object.
//
local config = {
  local comps = util.consts.components,

  // Instance specifics
  instance: {
    name: "K8s Prow",
    botName: "k8s-ci-robot",
    url: "https://prow.k8s.io",
    monitoringURL: "https://monitoring.prow.k8s.io",
  },

  // SLO compliance tracking config
  slo: {
    components: [
      comps.deck,
      comps.hook,
      comps.prowControllerManager,
      comps.sinker,
      comps.tide,
      comps.monitoring,
    ],
  },

  ciAbsents: {
    components: [
      comps.crier,
      comps.deck,
      comps.ghproxy,
      comps.hook,
      comps.horologium,
      comps.prowControllerManager,
      comps.sinker,
      comps.tide,
    ],
  },

  // Heartbeat jobs
  heartbeatJobs: [
    {name: 'ci-test-infra-prow-checkconfig', interval: '5m', alertInterval: '20m'},
  ],

  // Tide pools that are important enough to have their own graphs on the dashboard.
  tideDashboardExplicitPools: [
    {org: 'kubernetes', repo: 'kubernetes', branch: 'master'},
  ],

  // Additional scraping endpoints
  probeTargets: [
  # ATTENTION: Keep this in sync with the list in ../../additional-scrape-configs_secret.yaml
    {url: 'https://prow.k8s.io', labels: {slo: comps.deck}},
    {url: 'https://monitoring.prow.k8s.io', labels: {slo: comps.monitoring}},
    {url: 'https://testgrid.k8s.io', labels: {}},
    {url: 'https://gubernator.k8s.io', labels: {}},
    {url: 'https://gubernator.k8s.io/pr/fejta', labels: {}}, # Deep health check of someone's PR dashboard.
    {url: 'https://storage.googleapis.com/k8s-gubernator/triage/index.html', labels: {}},
    {url: 'https://storage.googleapis.com/test-infra-oncall/oncall.html', labels: {}},
  ],

  // Boskos endpoints to be monitored
  boskosResourcetypes: [
    # TODO(chaodaiG after 05/18/2021): drop instance. https://github.com/kubernetes/test-infra/pull/20888
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "aws-account", friendly: "AWS account"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "image-builder-aws-account", friendly: "Image Builder - AWS"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "gce-project", friendly: "GCE project"},
    {instance: "35.225.208.117:9090", job: "k8s-infra-prow-builds-boskos", type: "gce-project", friendly: "GCE project (k8s-infra)"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "gke-project", friendly: "GKE project"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "gpu-project", friendly: "GPU project"},
    {instance: "35.225.208.117:9090", job: "k8s-infra-prow-builds-boskos", type: "gpu-project", friendly: "GPU project (k8s-infra)"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "ingress-project", friendly: "Ingress project"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "node-e2e-project", friendly: "Node e2e project"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "scalability-project", friendly: "Scalability project"},
    {instance: "35.225.208.117:9090", job: "k8s-infra-prow-builds-boskos", type: "scalability-project", friendly: "Scalability project (k8s-infra)"},
    {instance: "104.197.27.114:9090", job: "k8s-prow-builds-new-boskos", type: "scalability-presubmit-project", friendly: "Scalability presubmit project"}
  ],

  // How long we go during work hours without seeing a webhook before alerting.
  webhookMissingAlertInterval: '10m',

  // How many days prow hasn't been bumped.
  prowImageStaleByDays: {daysStale: 7, eventDuration: '24h'},
};

// Generate the real config by adding in constant fields and defaulting where needed.
{
  _config+:: util.defaultConfig(config),
  _util+:: util,
}
