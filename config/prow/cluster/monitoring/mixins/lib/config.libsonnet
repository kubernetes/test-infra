{
  _config+:: {
    // Grafana dashboard IDs are necessary for stable links for dashboards
    grafanaDashboardIDs: {
      'boskos-http.json': 'eec46c579cbf4a518e5bbcbbf4913de9',
      'ghproxy.json': 'd72fe8d0400b2912e319b1e95d0ab1b3',
      'slo.json': 'ea313af4b7904c7c983d20d9572235a5',
    },
    // Component name constants
    components: {
      // Values should be lowercase for use with prometheus 'job' label.
      crier: 'crier',
      deck: 'deck',
      ghproxy: 'ghproxy',
      hook: 'hook',
      horologium: 'horologium',
      monitoring: 'monitoring', // Aggregate of prometheus, alertmanager, and grafana.
      plank: 'plank', // Mutually exclusive with prowControllerManager
      prowControllerManager: 'prow-controller-manager',
      sinker: 'sinker',
      tide: 'tide',
    },
    local comps = self.components,

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
    ],

    // How long we go during work hours without seeing a webhook before alerting.
    webhookMissingAlertInterval: '10m',
  },
}
