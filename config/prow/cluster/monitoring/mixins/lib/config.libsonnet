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
        comps.sinker,
        comps.tide,
        comps.monitoring,
      ],
    }
  },
}
