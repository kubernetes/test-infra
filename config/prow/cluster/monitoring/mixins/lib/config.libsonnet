{
  _config+:: {
    // Grafana dashboard IDs are necessary for stable links for dashboards
    grafanaDashboardIDs: {
      'boskos-http.json': 'eec46c579cbf4a518e5bbcbbf4913de9',
      'ghproxy.json': 'd72fe8d0400b2912e319b1e95d0ab1b3',
      'slo.json': 'ea313af4b7904c7c983d20d9572235a5',
    },
    components: {
      tide: 'Tide'
    },
    local components = self.components,
    slo: {
      components: [components.tide],
    }
  },
}
